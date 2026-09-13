package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

type computeSnapshots struct {
	snapshot port.SignalSnapshot
	requests []port.SnapshotKey
	pages    []port.PageRequest
	err      error
}

func (f *computeSnapshots) Latest(_ context.Context, key port.SnapshotKey, page port.PageRequest) (port.SignalSnapshot, error) {
	f.requests = append(f.requests, key)
	f.pages = append(f.pages, page)
	if f.err != nil {
		return port.SignalSnapshot{}, f.err
	}
	if f.snapshot.ID == "" || key.ParametersHash != f.snapshot.Key.ParametersHash {
		return port.SignalSnapshot{}, port.ErrSnapshotNotReady
	}
	out := f.snapshot
	start := int(page.AfterSequence)
	end := min(start+page.Limit, len(out.Rows))
	out.Rows = out.Rows[start:end]
	return out, nil
}

type changedMarket struct {
	*computeMarket
	change port.MarketChangeSet
	err    error
}

func (m *changedMarket) MarketChanges(context.Context, market.DataVersion, market.DataVersion) (port.MarketChangeSet, error) {
	return m.change, m.err
}
func scanRequest() ScanRequest {
	return ScanRequest{StrategyID: "test", StrategyVersion: "1", IdempotencyKey: "scan", From: marketDate(1), AsOf: marketDate(3), Scope: port.InstrumentScope{Limit: 5000}}
}
func newScanFixture(t *testing.T, ids []market.InstrumentID) (*ScanService, *changedMarket, *computeStore, *computeSnapshots) {
	t.Helper()
	m := &changedMarket{computeMarket: &computeMarket{version: 7, quality: port.DataComplete, ids: ids}}
	st := &computeStore{}
	snap := &computeSnapshots{}
	s, err := NewScanService(computeRegistry(t), m, st, st, snap, ScanConfig{ComputeConfig: ComputeConfig{EngineVersion: "1"}, Workers: 4})
	require.NoError(t, err)
	return s, m, st, snap
}

func TestScanLoadsEachTimeframeInBoundedBatches(t *testing.T) {
	ids := make([]market.InstrumentID, 5000)
	for i := range ids {
		ids[i] = market.InstrumentID{Exchange: market.SSE, Code: fmt.Sprintf("%06d", 5000-i)}
	}
	s, m, st, _ := newScanFixture(t, ids)
	_, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.LessOrEqual(t, m.calls, 4)
	require.Len(t, st.snapshot.Rows, 5000)
	for i := 1; i < len(st.snapshot.Rows); i++ {
		require.Less(t, st.snapshot.Rows[i-1].Instrument.String(), st.snapshot.Rows[i].Instrument.String())
	}
}
func TestScanPublishesFailuresWithoutDroppingSuccessfulSignals(t *testing.T) {
	other := market.InstrumentID{Exchange: market.SSE, Code: "600001"}
	s, m, st, _ := newScanFixture(t, []market.InstrumentID{marketID, other})
	m.fail = map[market.InstrumentID]error{other: errors.New("bad adjustment password=secret")}
	_, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Equal(t, port.RunPartialSucceeded, st.run.Status)
	require.Len(t, st.snapshot.Rows, 1)
	require.Len(t, st.snapshot.Failures, 1)
	require.NotContains(t, st.snapshot.Failures[other].Message, "secret")
}
func TestScanIncrementalPinsPagesAndInvalidatesFactors(t *testing.T) {
	ids := make([]market.InstrumentID, 1100)
	for i := range ids {
		ids[i] = market.InstrumentID{Exchange: market.SSE, Code: fmt.Sprintf("%06d", i)}
	}
	s, m, st, snap := newScanFixture(t, ids)
	_, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	snap.snapshot = st.snapshot
	st.run = port.Run{}
	m.version = 8
	m.change.Dirty = []market.InstrumentID{ids[0]}
	m.fail = map[market.InstrumentID]error{ids[0]: errors.New("invalid")}
	_, err = s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Len(t, st.snapshot.Rows, 1099)
	require.Len(t, st.snapshot.Failures, 1)
	for i, p := range snap.pages {
		if p.AfterSequence > 0 {
			require.Equal(t, snap.snapshot.ID, snap.requests[i].SnapshotID)
		}
	}
	st.run = port.Run{}
	m.version = 9
	m.change.FactorsOrActionsChanged = true
	m.fail = nil
	_, err = s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Len(t, st.snapshot.Rows, 1100)
}
func TestScanFreshStrategiesAndCancellation(t *testing.T) {
	s, _, st, _ := newScanFixture(t, []market.InstrumentID{marketID, {Exchange: market.SSE, Code: "600001"}})
	var made atomic.Int64
	s.registry = &strategy.Registry{}
	require.NoError(t, s.registry.Register("test", "1", func(map[string]float64) (strategy.Strategy, error) {
		made.Add(1)
		return &computeStrategy{definition: strategy.Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day}, onBar: func(c strategy.Context) (strategy.Decision, error) {
			return strategy.Decision{Action: strategy.Hold}, nil
		}}, nil
	}))
	_, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	before := made.Load()
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.GreaterOrEqual(t, made.Load()-before, int64(2))
	require.Empty(t, st.snapshot.Rows)
	require.NoError(t, s.Cancel(context.Background(), st.run.ID))
	require.Equal(t, port.RunCancelled, st.run.Status)
}

func TestScanIdempotencyKeepsOriginalUniverseAndRejectsInputConflict(t *testing.T) {
	s, m, _, _ := newScanFixture(t, []market.InstrumentID{marketID})
	request := scanRequest()
	first, err := s.Create(context.Background(), request)
	require.NoError(t, err)
	m.version = 8
	m.ids = nil
	second, err := s.Create(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	request.From = marketDate(2)
	_, err = s.Create(context.Background(), request)
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	request = scanRequest()
	request.Parameters = map[string]float64{"unknown": 1}
	_, err = s.Create(context.Background(), request)
	require.ErrorIs(t, err, strategy.ErrUnknownParameter)
}
func TestScanRetainsDeterministicStrategyFailuresAndMissingRows(t *testing.T) {
	for _, action := range []strategy.Action{strategy.EnterLong, strategy.Action(200)} {
		s, _, st, _ := newScanFixture(t, []market.InstrumentID{marketID})
		s.registry = &strategy.Registry{}
		require.NoError(t, s.registry.Register("test", "1", func(map[string]float64) (strategy.Strategy, error) {
			return &computeStrategy{definition: strategy.Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day}, onBar: func(strategy.Context) (strategy.Decision, error) { return strategy.Decision{Action: action}, nil }}, nil
		}))
		_, err := s.Create(context.Background(), scanRequest())
		require.NoError(t, err)
		require.NoError(t, s.Execute(context.Background(), st.claim()))
		require.Empty(t, st.snapshot.Rows)
		require.Len(t, st.snapshot.Failures, 1)
	}
}
func TestScanBaseFailuresAbortPublicationAndCancellationStopsBatch(t *testing.T) {
	for _, phase := range []string{"lookup", "changes", "lease", "context"} {
		s, m, st, snaps := newScanFixture(t, []market.InstrumentID{marketID})
		_, err := s.Create(context.Background(), scanRequest())
		require.NoError(t, err)
		require.NoError(t, s.Execute(context.Background(), st.claim()))
		snaps.snapshot = st.snapshot
		st.snapshot = port.SignalSnapshot{}
		st.run = port.Run{}
		m.version = 8
		_, err = s.Create(context.Background(), scanRequest())
		require.NoError(t, err)
		run := st.claim()
		ctx, cancel := context.WithCancel(context.Background())
		switch phase {
		case "lookup":
			snaps.err = port.ErrTemporary
		case "changes":
			m.err = port.ErrTemporary
		case "lease":
			st.run.LeaseToken = "new"
		case "context":
			cancel()
		}
		require.Error(t, s.Execute(ctx, run))
		cancel()
		require.Empty(t, st.snapshot.ID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := scanBatch(ctx, 2, []market.InstrumentID{marketID}, func(context.Context, market.InstrumentID) (port.SnapshotRow, bool, error) {
		t.Fatal("cancelled scan called strategy")
		return port.SnapshotRow{}, false, nil
	})
	require.Empty(t, result.rows)
}

func TestScanPinsBaseSnapshotAndMicrosecondAsOf(t *testing.T) {
	s, _, st, snaps := newScanFixture(t, []market.InstrumentID{marketID})
	request := scanRequest()
	request.AsOf = request.AsOf.Add(123456789)
	_, err := s.Create(context.Background(), request)
	require.NoError(t, err)
	var saved scanInput
	require.NoError(t, json.Unmarshal(st.run.RequestJSON, &saved))
	require.Equal(t, 123456000, saved.AsOf.Nanosecond())
	run := st.claim()
	saved.PreviousSnapshotID = "tampered"
	run.RequestJSON, _ = json.Marshal(saved)
	before := len(snaps.requests)
	require.Error(t, s.Execute(context.Background(), run))
	require.Len(t, snaps.requests, before)
}

func TestScanNeverPublishesSignalsOutsidePinnedWindow(t *testing.T) {
	s, m, st, _ := newScanFixture(t, []market.InstrumentID{marketID})
	m.mutate = func(id market.InstrumentID, b *port.Bundle) {
		bars := b.Primary.Bars()
		bars[len(bars)-1].CloseTime = marketDate(4)
		b.Primary, _ = market.NewDataset(id, market.Day, 7, bars)
	}
	_, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	require.NoError(t, s.Execute(context.Background(), st.claim()))
	require.Empty(t, st.snapshot.Rows)
	require.Equal(t, "INCOMPLETE_DATA", st.snapshot.Failures[marketID].Code)
}
