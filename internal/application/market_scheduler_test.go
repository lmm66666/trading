package application

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

type refreshFunc func(context.Context, market.InstrumentID) (RefreshResult, error)

func (f refreshFunc) Refresh(c context.Context, id market.InstrumentID) (RefreshResult, error) {
	return f(c, id)
}
func schedulerFixture(t *testing.T, n int, f refreshFunc) (*MarketScheduler, *marketReadFake) {
	t.Helper()
	data := &marketReadFake{}
	for i := 0; i < n; i++ {
		data.ids = append(data.ids, market.InstrumentID{Exchange: market.SSE, Code: fmt.Sprintf("%06d", 600000+i)})
	}
	s, err := NewMarketScheduler(data, f, port.InstrumentScope{Limit: 5000})
	require.NoError(t, err)
	return s, data
}
func TestMarketSchedulerBoundedPoolAndPerInstrumentFailures(t *testing.T) {
	var active, peak atomic.Int64
	s, _ := schedulerFixture(t, 40, func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		if id == marketID {
			return RefreshResult{}, port.ErrTemporary
		}
		return RefreshResult{Instrument: id, Version: 1, Quality: port.DataComplete}, nil
	})
	got := s.RunOnce(context.Background(), 3)
	require.NoError(t, got.Err)
	require.Equal(t, 40, got.Total)
	require.Len(t, got.Results, 39)
	require.ErrorIs(t, got.Failures[marketID], port.ErrTemporary)
	require.LessOrEqual(t, peak.Load(), int64(3))
	last := s.LastSummary()
	delete(last.Failures, marketID)
	require.Len(t, s.LastSummary().Failures, 1)
}
func TestMarketSchedulerOverlapAndCancellation(t *testing.T) {
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, _ := schedulerFixture(t, 5, func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
		close(entered)
		<-ctx.Done()
		return RefreshResult{}, ctx.Err()
	})
	done := make(chan RefreshSummary, 1)
	go func() { done <- s.RunOnce(ctx, 1) }()
	<-entered
	require.ErrorIs(t, s.RunOnce(context.Background(), 1).Err, ErrRefreshAlreadyRunning)
	cancel()
	got := <-done
	require.ErrorIs(t, got.Err, context.Canceled)
	require.Len(t, got.Failures, 5)
}
func TestMarketSchedulerStartStopsAndRejectsDuplicateLifecycle(t *testing.T) {
	entered := make(chan struct{}, 2)
	s, _ := schedulerFixture(t, 1, func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
		select {
		case entered <- struct{}{}:
		case <-ctx.Done():
			return RefreshResult{}, ctx.Err()
		}
		return RefreshResult{Instrument: id, Version: 1, Quality: port.DataComplete}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx, time.Millisecond*10, 1) }()
	<-entered
	require.ErrorIs(t, s.Start(context.Background(), time.Hour, 1), ErrRefreshAlreadyRunning)
	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.ErrorIs(t, s.Start(context.Background(), 0, 1), ErrInvalidRequest)
	require.ErrorIs(t, s.RunOnce(context.Background(), 0).Err, ErrInvalidRequest)
}
func TestMarketSchedulerInputAndUniverseFailures(t *testing.T) {
	s, data := schedulerFixture(t, 1, func(context.Context, market.InstrumentID) (RefreshResult, error) { return RefreshResult{}, nil })
	data.errorRead = port.ErrTemporary
	require.ErrorIs(t, s.RunOnce(context.Background(), 1).Err, port.ErrTemporary)
	data.errorRead = nil
	data.ids = append(data.ids, data.ids[0])
	got := s.RunOnce(context.Background(), 2)
	require.Equal(t, 1, got.Total)
	data.ids = []market.InstrumentID{{}}
	require.ErrorIs(t, s.RunOnce(context.Background(), 1).Err, ErrInvalidRequest)
}
func TestMarketSchedulerLifecycleErrorAndEmptyUniverse(t *testing.T) {
	s, data := schedulerFixture(t, 0, func(context.Context, market.InstrumentID) (RefreshResult, error) {
		t.Fatal("unexpected refresh")
		return RefreshResult{}, nil
	})
	require.NoError(t, s.RunOnce(context.Background(), 1).Err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, s.RunOnce(ctx, 1).Err, context.Canceled)
	data.errorRead = port.ErrTemporary
	require.ErrorIs(t, s.Start(context.Background(), time.Hour, 1), port.ErrTemporary)
	data.errorRead = nil
	s.scope.Limit = 1
	data.ids = []market.InstrumentID{marketID, marketID}
	require.ErrorIs(t, s.RunOnce(context.Background(), 1).Err, ErrInvalidRequest)
	_, err := NewMarketScheduler(nil, s.refresher, port.InstrumentScope{Limit: 1})
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, err = NewMarketScheduler(data, s.refresher, port.InstrumentScope{})
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestMarketSchedulerTriggerNowIsAsyncAndWaitsForShutdown(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	s, _ := schedulerFixture(t, 1, func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
		close(entered)
		select {
		case <-release:
			return RefreshResult{Instrument: id, Version: 1, Quality: port.DataComplete}, nil
		case <-ctx.Done():
			return RefreshResult{}, ctx.Err()
		}
	})
	require.NoError(t, s.TriggerNow(context.Background(), 1))
	<-entered
	require.ErrorIs(t, s.TriggerNow(context.Background(), 1), ErrRefreshAlreadyRunning)
	waited := make(chan struct{})
	go func() { s.Wait(); close(waited) }()
	select {
	case <-waited:
		t.Fatal("Wait returned while refresh was still running")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not observe async refresh completion")
	}
}
