package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
	"trading/internal/backtest"
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

type failedEngine struct{ err error }

func (e failedEngine) Run(context.Context, backtest.Input) (backtest.Result, error) {
	return backtest.Result{}, e.err
}
func TestBacktestRejectsTamperedSavedInputsBeforeLoading(t *testing.T) {
	for _, change := range []func(*port.Run){func(r *port.Run) { r.RequestJSON = []byte(`{"x":`) }, func(r *port.Run) { r.RequestJSON = []byte(`{}`) }, func(r *port.Run) { r.EngineVersion = "different" }, func(r *port.Run) { r.InputHash = "different" }, func(r *port.Run) { r.Kind = port.RunScan }, func(r *port.Run) { r.Attempts = 0 }, func(r *port.Run) {
		var v BacktestRequest
		_ = json.Unmarshal(r.RequestJSON, &v)
		v.StrategyID = "unknown"
		r.RequestJSON, _ = json.Marshal(v)
	}} {
		s, m, st := newBacktestFixture(t)
		_, err := s.Create(context.Background(), computeRequest())
		require.NoError(t, err)
		r := st.claim()
		change(&r)
		require.Error(t, s.Execute(context.Background(), r))
		require.Zero(t, m.calls)
	}
}
func TestBacktestPropagatesEngineFailureAndCancellation(t *testing.T) {
	s, m, st := newBacktestFixture(t)
	_, err := s.Create(context.Background(), computeRequest())
	require.NoError(t, err)
	r := st.claim()
	s.engine = failedEngine{port.ErrTemporary}
	require.ErrorIs(t, s.Execute(context.Background(), r), port.ErrTemporary)
	require.Empty(t, st.result.Equity)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, s.Execute(ctx, r), context.Canceled)
	_, err = s.Create(ctx, computeRequest())
	require.ErrorIs(t, err, context.Canceled)
	m.version = 0
	st.run = port.Run{}
	_, err = s.Create(context.Background(), computeRequest())
	require.ErrorIs(t, err, port.ErrMarketDataNotFound)
}
func TestComputeConstructorAndRequestBoundaries(t *testing.T) {
	s, m, st := newBacktestFixture(t)
	_, err := NewBacktestService(nil, s.engine, m, st, st, ComputeConfig{EngineVersion: "1"})
	require.Error(t, err)
	_, err = NewBacktestService(s.registry, s.engine, m, st, st, ComputeConfig{})
	require.Error(t, err)
	r := computeRequest()
	r.Start = time.Time{}
	_, err = s.Create(context.Background(), r)
	require.Error(t, err)
	r = computeRequest()
	r.IdempotencyKey = string(make([]byte, 200))
	_, err = s.Create(context.Background(), r)
	require.Error(t, err)
	_, err = canonicalJSON(math.Inf(1))
	require.Error(t, err)
	_, err = computeHash(math.NaN())
	require.Error(t, err)
	data, err := canonicalJSON(map[string]any{"z": float64(1e-9), "a": uint64(math.MaxUint64), "n": math.Copysign(0, -1)})
	require.NoError(t, err)
	require.JSONEq(t, `{"a":18446744073709551615,"n":0,"z":0.000000001}`, string(data))
	require.NotContains(t, string(data), "e-")
}

func TestBacktestBuildsAdjustedAndAuxiliaryFeatures(t *testing.T) {
	s, m, st := newBacktestFixture(t)
	s.registry = &strategy.Registry{}
	def := strategy.Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Auxiliary: []market.Timeframe{market.Week}, Features: []indicator.Ref{{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close}, {Kind: indicator.OHLC, Timeframe: market.Week, Field: indicator.Close}}}
	require.NoError(t, s.registry.Register("test", "1", func(map[string]float64) (strategy.Strategy, error) {
		return &computeStrategy{definition: def, onBar: func(c strategy.Context) (strategy.Decision, error) {
			v, ok := c.Float(def.Features[0], 0)
			if !ok || v != 20 {
				return strategy.Decision{}, errors.New("wrong adjustment")
			}
			if c.Index() > 0 {
				v, ok = c.Float(def.Features[1], 0)
				if !ok || v != 10 {
					return strategy.Decision{}, errors.New("wrong auxiliary alignment")
				}
			}
			return strategy.Decision{Action: strategy.Hold}, nil
		}}, nil
	}))
	m.mutate = func(id market.InstrumentID, b *port.Bundle) {
		bar := marketBar(market.Week, 1)
		bar.Instrument = id
		bar.Version = 7
		d, _ := market.NewDataset(id, market.Week, 7, []market.Bar{bar})
		b.Auxiliary = map[market.Timeframe]market.Dataset{market.Week: d}
		b.Factors = []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Version: 7, Numerator: 2, Denominator: 1}}
	}
	_, err := s.Create(context.Background(), computeRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Len(t, st.result.Equity, 3)
}
func TestComputeBundleFailsClosedForMalformedVersionedData(t *testing.T) {
	m := &computeMarket{quality: port.DataComplete}
	bundles, _ := m.BatchDatasets(context.Background(), []market.InstrumentID{marketID}, port.BatchRequest{PrimaryTimeframe: market.Day, Version: 7})
	valid := bundles[marketID]
	def := strategy.Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day}
	for _, test := range []struct {
		name   string
		change func(*port.Bundle, *strategy.Definition)
	}{
		{"invalid quality", func(b *port.Bundle, _ *strategy.Definition) { b.Quality = "invalid" }},
		{"missing auxiliary", func(_ *port.Bundle, d *strategy.Definition) { d.Auxiliary = []market.Timeframe{market.Week} }},
		{"wrong primary", func(_ *port.Bundle, d *strategy.Definition) { d.PrimaryTimeframe = market.Week }},
		{"wrong version", func(b *port.Bundle, _ *strategy.Definition) {
			bars := b.Primary.Bars()
			for i := range bars {
				bars[i].Version = 8
			}
			b.Primary, _ = market.NewDataset(marketID, market.Day, 8, bars)
		}},
		{"non UTC bar", func(b *port.Bundle, _ *strategy.Definition) {
			bars := b.Primary.Bars()
			bars[0].OpenTime = bars[0].OpenTime.In(time.FixedZone("offset", 3600))
			b.Primary, _ = market.NewDataset(marketID, market.Day, 7, bars)
		}},
		{"wrong factor", func(b *port.Bundle, _ *strategy.Definition) {
			b.Factors = []market.AdjustmentFactor{{Version: 8, Numerator: 1, Denominator: 1}}
		}},
		{"duplicate factor", func(b *port.Bundle, _ *strategy.Definition) {
			f := market.AdjustmentFactor{EffectiveTime: marketDate(1), Version: 7, Numerator: 1, Denominator: 1}
			b.Factors = []market.AdjustmentFactor{f, f}
		}},
		{"wrong action", func(b *port.Bundle, _ *strategy.Definition) {
			b.Actions = []market.CorporateAction{{Version: 8, Instrument: marketID}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, d := valid, def
			test.change(&b, &d)
			require.ErrorIs(t, validateComputeBundle(b, marketID, d, 7), ErrIncompleteMarketData)
		})
	}
	def.Features = []indicator.Ref{{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close}}
	_, err := buildComputeTimeline(valid, def)
	require.Error(t, err)
}

func TestScanValidationQueryPaginationAndSavedInputs(t *testing.T) {
	s, m, st, snaps := newScanFixture(t, []market.InstrumentID{marketID})
	for _, change := range []func(*ScanRequest){func(r *ScanRequest) { r.Scope.Limit = 0 }, func(r *ScanRequest) { r.From = time.Time{} }, func(r *ScanRequest) { r.AsOf = time.Time{} }, func(r *ScanRequest) { r.AsOf = r.From.AddDate(21, 0, 0) }, func(r *ScanRequest) { r.IdempotencyKey = "" }} {
		r := scanRequest()
		change(&r)
		_, err := s.Create(context.Background(), r)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Create(ctx, scanRequest())
	require.ErrorIs(t, err, context.Canceled)
	m.version = 0
	_, err = s.Create(context.Background(), scanRequest())
	require.ErrorIs(t, err, port.ErrMarketDataNotFound)
	m.version = 7
	snaps.err = port.ErrTemporary
	_, err = s.Create(context.Background(), scanRequest())
	require.ErrorIs(t, err, port.ErrTemporary)
	snaps.err = nil
	_, err = s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	run := st.claim()
	run.InputHash = "wrong"
	require.Error(t, s.Execute(context.Background(), run))
	require.Zero(t, m.calls)
	run = st.run
	run.RequestJSON = []byte(`{}`)
	require.Error(t, s.Execute(context.Background(), run))
	run = st.run
	run.Kind = port.RunBacktest
	require.Error(t, s.Execute(context.Background(), run))
	_, err = s.Latest(context.Background(), port.SnapshotKey{}, port.PageRequest{Limit: 1})
	require.Error(t, err)
	key := port.SnapshotKey{StrategyID: "test", StrategyVersion: "1", ParametersHash: "hash"}
	_, err = s.Latest(context.Background(), key, port.PageRequest{})
	require.Error(t, err)
	_, err = s.Latest(context.Background(), key, port.PageRequest{AfterSequence: 1, Limit: 1})
	require.Error(t, err)
	_, err = s.Latest(context.Background(), key, port.PageRequest{Limit: 1})
	require.ErrorIs(t, err, port.ErrSnapshotNotReady)
	_, err = NewScanService(nil, m, st, st, snaps, ScanConfig{})
	require.Error(t, err)
	_, err = NewScanService(s.registry, m, st, st, snaps, ScanConfig{})
	require.Error(t, err)
	_, err = NewScanService(s.registry, m, st, st, snaps, ScanConfig{ComputeConfig: ComputeConfig{EngineVersion: "1"}})
	require.Error(t, err)
}
func TestScanCanonicalUniverseAndConfigInvalidateSnapshot(t *testing.T) {
	s, m, st, snap := newScanFixture(t, []market.InstrumentID{marketID, marketID})
	r, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Len(t, st.snapshot.Rows, 1)
	snap.snapshot = st.snapshot
	st.run = port.Run{}
	request := scanRequest()
	request.Parameters = map[string]float64{"a": 2}
	r2, err := s.Create(context.Background(), request)
	require.NoError(t, err)
	require.NotEqual(t, r.InputHash, r2.InputHash)
	var saved scanInput
	require.NoError(t, json.Unmarshal(r2.RequestJSON, &saved))
	require.Empty(t, saved.PreviousSnapshotID)
	st.run = port.Run{}
	m.ids = []market.InstrumentID{{}}
	_, err = s.Create(context.Background(), request)
	require.Error(t, err)
	_, err = canonicalScanIDs([]market.InstrumentID{marketID, marketID}, 1)
	require.Error(t, err)
}
