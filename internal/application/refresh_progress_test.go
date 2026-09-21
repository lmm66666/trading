package application

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

type progressMemory struct {
	mu        sync.Mutex
	snapshots []port.RefreshSnapshot
	fail      bool
}

func (m *progressMemory) SaveRefresh(_ context.Context, s port.RefreshSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("private database failure")
	}
	m.snapshots = append(m.snapshots, s)
	return nil
}
func TestRefreshProgressCountsFailuresWithoutExposingErrors(t *testing.T) {
	store := &progressMemory{}
	p := NewRefreshProgress(store)
	h, receipt := p.Begin(context.Background(), "STOCK", "MANUAL")
	require.True(t, receipt.ProgressAvailable)
	require.NotEmpty(t, receipt.RunID)
	h.Prepared(3)
	h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600000"}, nil)
	h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600001"}, errors.New("password=secret"))
	h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600002"}, context.Canceled)
	h.End(context.Canceled)
	p.Flush(context.Background())
	got := store.snapshots[len(store.snapshots)-1]
	require.Equal(t, "INTERRUPTED", got.Run.State)
	require.Equal(t, 1, got.Run.Succeeded)
	require.Equal(t, 1, got.Run.Failed)
	require.Len(t, got.Failures, 1)
	require.Equal(t, "REFRESH_FAILED", got.Failures[0].ErrorCode)
	require.NotNil(t, got.Run.FinishedAt)
}
func TestRefreshProgressStoreFailureDoesNotStopCollection(t *testing.T) {
	store := &progressMemory{fail: true}
	p := NewRefreshProgress(store)
	h, receipt := p.Begin(context.Background(), "STOCK", "MANUAL")
	require.False(t, receipt.ProgressAvailable)
	h.Prepared(1)
	h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600000"}, nil)
	h.End(nil)
	store.fail = false
	p.Flush(context.Background())
	require.Equal(t, "SUCCEEDED", store.snapshots[0].Run.State)
}
func TestSchedulerTracksProgressWithoutSkippingPreviouslyProcessedSecurities(t *testing.T) {
	n := 0
	s, _ := schedulerFixture(t, 2, func(context.Context, market.InstrumentID) (RefreshResult, error) { n++; return RefreshResult{}, nil })
	store := &progressMemory{}
	p := NewRefreshProgress(store)
	s.SetProgress(p)
	for i := 0; i < 2; i++ {
		receipt, err := s.TriggerTracked(context.Background(), 1)
		require.NoError(t, err)
		require.NotEmpty(t, receipt.RunID)
		s.Wait()
	}
	p.Flush(context.Background())
	require.Equal(t, 4, n)
}
func TestRefreshProgressTerminalStates(t *testing.T) {
	for _, tc := range []struct {
		name             string
		total, good, bad int
		err              error
		state            string
	}{{"empty", 0, 0, 0, nil, "SUCCEEDED"}, {"mixed", 2, 1, 1, nil, "PARTIAL_SUCCEEDED"}, {"failed", 1, 0, 1, nil, "FAILED"}, {"prepare", 0, 0, 0, errors.New("private"), "FAILED"}} {
		t.Run(tc.name, func(t *testing.T) {
			m := &progressMemory{}
			p := NewRefreshProgress(m)
			h, _ := p.Begin(context.Background(), "FUTURES", "SCHEDULED")
			h.Prepared(tc.total)
			for i := 0; i < tc.good; i++ {
				h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600000"}, nil)
			}
			for i := 0; i < tc.bad; i++ {
				h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600001"}, errors.New("private"))
			}
			h.End(tc.err)
			p.Flush(context.Background())
			require.Equal(t, tc.state, m.snapshots[len(m.snapshots)-1].Run.State)
		})
	}
	p := NewRefreshProgress(&progressMemory{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, p.Run(ctx), context.Canceled)
	var h *RefreshObservation
	h.Prepared(1)
	h.Completed(market.InstrumentID{}, nil)
	h.End(nil)
}

func TestProgressHeartbeatAndConcurrentFlush(t *testing.T) {
	m := &progressMemory{}
	p := NewRefreshProgress(m)
	h, _ := p.Begin(context.Background(), "STOCK", "MANUAL")
	h.Prepared(100)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	for i := 0; i < 100; i++ {
		h.Completed(market.InstrumentID{Exchange: market.SSE, Code: "600000"}, nil)
	}
	h.End(nil)
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.snapshots) > 1 && m.snapshots[len(m.snapshots)-1].Run.State == "SUCCEEDED"
	}, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	p.Flush(ctx)
}
func TestProgressBoundsAndCanceledFlush(t *testing.T) {
	m := &progressMemory{fail: true}
	p := NewRefreshProgress(m)
	for i := 0; i < 64; i++ {
		h, _ := p.Begin(context.Background(), "STOCK", "MANUAL")
		h.End(nil)
	}
	h, r := p.Begin(context.Background(), "STOCK", "MANUAL")
	require.Nil(t, h)
	require.False(t, r.ProgressAvailable)
	require.Len(t, p.pending, 64)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Flush(ctx)
	var empty *RefreshProgress
	empty.Flush(ctx)
	h, r = empty.Begin(ctx, "STOCK", "MANUAL")
	require.Nil(t, h)
	require.NotEmpty(t, r.RunID)
}
func TestFuturesProgressUsesSeparateScope(t *testing.T) {
	store := &progressMemory{}
	p := NewRefreshProgress(store)
	fake := refreshFunc(func(context.Context, market.InstrumentID) (RefreshResult, error) { return RefreshResult{}, nil })
	s, err := NewFuturesScheduler(fake, DefaultSinaFuturesInstruments(), slog.Default())
	require.NoError(t, err)
	s.SetProgress(p)
	summary := s.RunOnce(context.Background())
	require.NoError(t, summary.Err)
	p.Flush(context.Background())
	got := store.snapshots[len(store.snapshots)-1].Run
	require.Equal(t, "FUTURES", got.Kind)
	require.Equal(t, 8, got.Succeeded)
}
func TestProgressRecoveryRetriesUntilSuccessful(t *testing.T) {
	p := NewRefreshProgress(&progressMemory{})
	attempts := 0
	p.SetRecovery(func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary")
		}
		return nil
	})
	p.Flush(context.Background())
	p.Flush(context.Background())
	p.Flush(context.Background())
	require.Equal(t, 2, attempts)
}

func TestRefreshProgressFiveHourRunKeepsHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &progressMemory{}
		p := NewRefreshProgress(store)
		scheduler, _ := schedulerFixture(t, 1, func(context.Context, market.InstrumentID) (RefreshResult, error) {
			time.Sleep(5 * time.Hour)
			return RefreshResult{}, nil
		})
		scheduler.SetProgress(p)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- p.Run(ctx) }()
		receipt, err := scheduler.TriggerTracked(ctx, 1)
		require.NoError(t, err)
		require.True(t, receipt.ProgressAvailable)
		time.Sleep(4 * time.Hour)
		synctest.Wait()
		store.mu.Lock()
		last := store.snapshots[len(store.snapshots)-1].Run
		store.mu.Unlock()
		require.Equal(t, "RUNNING", last.State)
		require.Zero(t, last.Succeeded)
		require.LessOrEqual(t, time.Since(last.HeartbeatAt), 10*time.Second)
		time.Sleep(time.Hour)
		scheduler.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		p.Flush(context.Background())
		store.mu.Lock()
		last = store.snapshots[len(store.snapshots)-1].Run
		store.mu.Unlock()
		require.Equal(t, "SUCCEEDED", last.State)
		require.Equal(t, 1, last.Succeeded)
		require.Equal(t, 5*time.Hour, last.FinishedAt.Sub(last.StartedAt))
	})
}
