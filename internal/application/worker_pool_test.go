package application

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"trading/internal/port"
)

type poolQueue struct {
	port.JobQueue
	mu       sync.Mutex
	runs     []port.Run
	renewErr error
	renewed  chan struct{}
	reaped   atomic.Int64
}

func (q *poolQueue) Claim(ctx context.Context, _ string, _ time.Duration) (port.Run, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.runs) == 0 {
		return port.Run{}, port.ErrRunNotFound
	}
	r := q.runs[0]
	q.runs = q.runs[1:]
	return r, nil
}
func (q *poolQueue) Renew(context.Context, string, string, time.Duration) error {
	if q.renewed != nil {
		select {
		case q.renewed <- struct{}{}:
		default:
		}
	}
	return q.renewErr
}
func (q *poolQueue) ReapExpired(context.Context) (int64, error) { q.reaped.Add(1); return 0, nil }
func TestWorkerPoolPersistsRetryScheduleAndFourthFailure(t *testing.T) {
	for _, attempt := range []int{1, 2, 3, port.MaxRunAttempts} {
		s, _, store := newBacktestFixture(t)
		_, err := s.Create(context.Background(), computeRequest())
		require.NoError(t, err)
		run := store.claim()
		run.Attempts = attempt
		now := marketDate(1)
		pool, err := NewWorkerPool(store, store, map[port.RunKind]RunHandler{port.RunBacktest: func(context.Context, port.Run) error { return port.ErrTemporary }}, WorkerPoolConfig{Owner: "test", Workers: 1, Lease: time.Second, PollInterval: time.Millisecond, Clock: func() time.Time { return now }})
		require.NoError(t, err)
		require.NoError(t, pool.process(context.Background(), run))
		if attempt < port.MaxRunAttempts {
			require.Equal(t, port.RunPending, store.run.Status)
			require.Equal(t, now.Add([]time.Duration{250 * time.Millisecond, time.Second, 4 * time.Second}[attempt-1]), store.retryTime)
		} else {
			require.Equal(t, port.RunFailed, store.run.Status)
			require.False(t, store.failure.Retryable)
		}
	}
}
func TestWorkerPoolLeaseLossCancelsExecutionWithoutPublishingFailure(t *testing.T) {
	s, _, store := newBacktestFixture(t)
	_, err := s.Create(context.Background(), computeRequest())
	require.NoError(t, err)
	run := store.claim()
	q := &poolQueue{renewErr: port.ErrLeaseLost}
	pool, err := NewWorkerPool(q, store, map[port.RunKind]RunHandler{port.RunBacktest: func(ctx context.Context, _ port.Run) error { <-ctx.Done(); return ctx.Err() }}, WorkerPoolConfig{Owner: "owner", Workers: 1, Lease: 3 * time.Millisecond, PollInterval: time.Millisecond})
	require.NoError(t, err)
	require.NoError(t, pool.process(context.Background(), run))
	require.Empty(t, store.failure.Code)
}
func TestWorkerPoolBoundsConcurrencyAndStopsAllWorkers(t *testing.T) {
	q := &poolQueue{}
	store := &computeStore{}
	for i := 0; i < 20; i++ {
		q.runs = append(q.runs, port.Run{ID: "run", Kind: port.RunBacktest})
	}
	var active, maxActive atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := func(ctx context.Context, _ port.Run) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		if n == 3 {
			cancel()
		}
		<-ctx.Done()
		return ctx.Err()
	}
	pool, err := NewWorkerPool(q, store, map[port.RunKind]RunHandler{port.RunBacktest: handler}, WorkerPoolConfig{Owner: "owner", Workers: 3, Lease: time.Hour, PollInterval: time.Millisecond})
	require.NoError(t, err)
	require.ErrorIs(t, pool.Run(ctx), context.Canceled)
	require.Equal(t, int64(0), active.Load())
	require.LessOrEqual(t, maxActive.Load(), int64(3))
	require.Positive(t, q.reaped.Load())
}
func TestWorkerPoolValidationAndTerminalErrors(t *testing.T) {
	store := &computeStore{}
	_, err := NewWorkerPool(store, store, nil, WorkerPoolConfig{})
	require.Error(t, err)
	pool, err := NewWorkerPool(store, store, map[port.RunKind]RunHandler{port.RunBacktest: func(context.Context, port.Run) error { return errors.New("password=secret") }}, WorkerPoolConfig{Owner: "owner", Workers: 1, Lease: time.Second, PollInterval: time.Millisecond})
	require.NoError(t, err)
	require.NoError(t, pool.process(context.Background(), port.Run{ID: "run", Kind: port.RunBacktest, Attempts: 1}))
	require.Equal(t, "COMPUTE_FAILED", store.failure.Code)
	require.NotContains(t, store.failure.Message, "secret")
}
