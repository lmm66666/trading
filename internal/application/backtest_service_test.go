package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

type computeStore struct {
	port.RunStore
	port.JobQueue
	run         port.Run
	result      backtest.Result
	snapshot    port.SignalSnapshot
	completeErr error
	failure     port.Failure
	retryTime   time.Time
}

func (s *computeStore) Enqueue(_ context.Context, r port.Run) (port.Run, error) {
	if s.run.ID != "" {
		if s.run.InputHash != r.InputHash {
			return port.Run{}, port.ErrInvalidPortValue
		}
		return s.run, nil
	}
	s.run = r
	return r, nil
}
func (s *computeStore) Get(context.Context, string) (port.Run, error) { return s.run, nil }
func (s *computeStore) FindByIdempotency(_ context.Context, kind port.RunKind, key string) (port.Run, error) {
	if s.run.ID == "" || s.run.Kind != kind || s.run.IdempotencyKey != key {
		return port.Run{}, port.ErrRunNotFound
	}
	return s.run, nil
}
func (s *computeStore) CompleteBacktest(_ context.Context, id, token string, r backtest.Result) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	s.result = r
	s.run.Status = port.RunSucceeded
	return nil
}
func (s *computeStore) CompleteScan(_ context.Context, id, token string, v port.SignalSnapshot) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	s.snapshot = v
	s.run.Status = port.RunSucceeded
	if len(v.Failures) > 0 {
		s.run.Status = port.RunPartialSucceeded
	}
	return nil
}
func (s *computeStore) RequestCancel(context.Context, string) error {
	s.run.Status = port.RunCancelled
	return nil
}
func (s *computeStore) Fail(_ context.Context, id, token string, f port.Failure) error {
	s.failure = f
	s.run.Status = port.RunFailed
	return s.completeErr
}
func (s *computeStore) Retry(_ context.Context, id, token string, at time.Time, f port.Failure) error {
	s.failure = f
	s.retryTime = at
	s.run.Status = port.RunPending
	return s.completeErr
}
func (s *computeStore) claim() port.Run {
	s.run.Status = port.RunRunning
	s.run.LeaseOwner = "worker"
	s.run.LeaseToken = "lease"
	s.run.Attempts++
	return s.run
}

type computeMarket struct {
	port.MarketData
	version   market.DataVersion
	quality   port.DataQuality
	ids       []market.InstrumentID
	calls     int
	last      port.BatchRequest
	fail      map[market.InstrumentID]error
	latestErr error
	mutate    func(market.InstrumentID, *port.Bundle)
}

func (m *computeMarket) LatestCompleteVersion(context.Context) (market.DataVersion, error) {
	return m.version, m.latestErr
}
func (m *computeMarket) Instruments(context.Context, port.InstrumentScope) ([]market.InstrumentID, error) {
	return m.ids, nil
}
func (m *computeMarket) DirtyInstruments(context.Context, market.DataVersion, market.DataVersion) ([]market.InstrumentID, error) {
	return nil, nil
}
func (m *computeMarket) BatchDatasets(_ context.Context, ids []market.InstrumentID, r port.BatchRequest) (map[market.InstrumentID]port.Bundle, map[market.InstrumentID]error) {
	m.calls++
	m.last = r
	out := map[market.InstrumentID]port.Bundle{}
	errs := map[market.InstrumentID]error{}
	for _, id := range ids {
		if err := m.fail[id]; err != nil {
			errs[id] = err
			continue
		}
		bars := []market.Bar{}
		for i := 1; i <= 3; i++ {
			b := marketBar(r.PrimaryTimeframe, i)
			b.Instrument = id
			b.Version = r.Version
			bars = append(bars, b)
		}
		d, _ := market.NewDataset(id, r.PrimaryTimeframe, r.Version, bars)
		v := port.Bundle{Primary: d, Quality: m.quality}
		if m.mutate != nil {
			m.mutate(id, &v)
		}
		out[id] = v
	}
	return out, errs
}

type computeStrategy struct {
	definition strategy.Definition
	calls      int
	onBar      func(strategy.Context) (strategy.Decision, error)
}

func (s *computeStrategy) Definition() strategy.Definition { return s.definition }
func (s *computeStrategy) OnBar(c strategy.Context) (strategy.Decision, error) {
	s.calls++
	if s.onBar != nil {
		return s.onBar(c)
	}
	return strategy.Decision{Action: strategy.EnterLong, Reason: "entry"}, nil
}
func computeRegistry(t *testing.T) *strategy.Registry {
	t.Helper()
	r := &strategy.Registry{}
	require.NoError(t, r.Register("test", "1", func(map[string]float64) (strategy.Strategy, error) {
		return &computeStrategy{definition: strategy.Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Parameters: map[string]strategy.ParameterSpec{"a": {Default: 1, Min: 0, Max: 10}, "b": {Default: 2, Min: 0, Max: 10}}}}, nil
	}))
	return r
}
func computeRequest() BacktestRequest {
	return BacktestRequest{Instrument: marketID, StrategyID: "test", StrategyVersion: "1", IdempotencyKey: "request", Start: marketDate(1), End: marketDate(3), Config: backtest.Config{InitialCash: 1000000000, CashFractionBPS: 10000, LotSize: 100}}
}
func newBacktestFixture(t *testing.T) (*BacktestService, *computeMarket, *computeStore) {
	t.Helper()
	m := &computeMarket{version: 7, quality: port.DataComplete}
	st := &computeStore{}
	s, err := NewBacktestService(computeRegistry(t), backtest.Engine{}, m, st, st, ComputeConfig{EngineVersion: "1"})
	require.NoError(t, err)
	return s, m, st
}

func TestCreateBacktestLocksAllReproductionInputs(t *testing.T) {
	s, m, st := newBacktestFixture(t)
	req := computeRequest()
	run, err := s.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, market.DataVersion(7), run.DataVersion)
	require.Len(t, run.InputHash, 64)
	require.Equal(t, "1", run.EngineVersion)
	var saved BacktestRequest
	require.NoError(t, json.Unmarshal(run.RequestJSON, &saved))
	require.Equal(t, map[string]float64{"a": 1, "b": 2}, saved.Parameters)
	duplicate, err := s.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, run.ID, duplicate.ID)
	m.version = 8
	duplicate, err = s.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, run.ID, duplicate.ID)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Equal(t, market.DataVersion(7), m.last.Version)
	require.Len(t, st.result.Equity, 3)
}
func TestBacktestCanonicalHashAndConflicts(t *testing.T) {
	r := computeRequest()
	d, _ := computeRegistry(t).Definition("test", "1")
	r.Parameters = map[string]float64{"b": 2, "a": 1}
	a, e := canonicalInputHash(r, d, 7, "1")
	require.NoError(t, e)
	r.Parameters = map[string]float64{"a": 1, "b": 2}
	b, e := canonicalInputHash(r, d, 7, "1")
	require.NoError(t, e)
	require.Equal(t, a, b)
	for _, change := range []func(*BacktestRequest){func(v *BacktestRequest) { v.Config.SlippageBPS++ }, func(v *BacktestRequest) { v.End = v.End.Add(time.Second) }, func(v *BacktestRequest) { v.Parameters["a"]++ }} {
		q := computeRequest()
		q.Parameters = map[string]float64{"a": 1, "b": 2}
		change(&q)
		h, e := canonicalInputHash(q, d, 7, "1")
		require.NoError(t, e)
		require.NotEqual(t, a, h)
	}
	s, _, _ := newBacktestFixture(t)
	_, e = s.Create(context.Background(), r)
	require.NoError(t, e)
	r.Config.SlippageBPS++
	_, e = s.Create(context.Background(), r)
	require.ErrorIs(t, e, port.ErrInvalidPortValue)
}
func TestBacktestRejectsQualityVersionAndLostLease(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*computeMarket, *computeStore)
		want error
	}{
		{"quality", func(m *computeMarket, _ *computeStore) { m.quality = port.DataIncomplete }, ErrIncompleteMarketData},
		{"missing", func(m *computeMarket, _ *computeStore) {
			m.fail = map[market.InstrumentID]error{marketID: port.ErrMarketDataNotFound}
		}, port.ErrMarketDataNotFound},
		{"lease", func(_ *computeMarket, st *computeStore) { st.completeErr = port.ErrLeaseLost }, port.ErrLeaseLost},
		{"cancel", func(_ *computeMarket, st *computeStore) { st.run.Status = port.RunCancelled }, context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, m, st := newBacktestFixture(t)
			_, err := s.Create(context.Background(), computeRequest())
			require.NoError(t, err)
			run := st.claim()
			test.edit(m, st)
			err = s.Execute(context.Background(), run)
			require.ErrorIs(t, err, test.want)
			require.Empty(t, st.result.Equity)
		})
	}
}
func TestBacktestCreateValidationAndCancel(t *testing.T) {
	s, m, st := newBacktestFixture(t)
	r := computeRequest()
	r.Parameters = map[string]float64{"unknown": 1}
	_, err := s.Create(context.Background(), r)
	require.ErrorIs(t, err, strategy.ErrUnknownParameter)
	r = computeRequest()
	m.latestErr = errors.New("storage")
	_, err = s.Create(context.Background(), r)
	require.Error(t, err)
	m.latestErr = nil
	_, err = s.Create(context.Background(), r)
	require.NoError(t, err)
	require.NoError(t, s.Cancel(context.Background(), st.run.ID))
	require.Equal(t, port.RunCancelled, st.run.Status)
}

func TestBacktestWarmupDoesNotTradeBeforeRequestedStart(t *testing.T) {
	s, _, st := newBacktestFixture(t)
	req := computeRequest()
	req.Start = marketDate(2)
	_, err := s.Create(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Len(t, st.result.Equity, 2)
	require.Equal(t, marketDate(2), st.result.Equity[0].Time)
}
