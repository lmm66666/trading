package application

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
	"trading/internal/port"
)

type fencedFinalizer struct {
	*poolQueue
	failure error
	writes  atomic.Int64
}

func (q *fencedFinalizer) Retry(context.Context, string, string, time.Time, port.Failure) error {
	q.writes.Add(1)
	return q.failure
}

type fencedStore struct {
	port.RunStore
	failure error
	writes  atomic.Int64
}

func (s *fencedStore) Fail(context.Context, string, string, port.Failure) error {
	s.writes.Add(1)
	return s.failure
}

func TestWorkerPoolContinuesAfterFinalizationLosesLease(t *testing.T) {
	for _, retry := range []bool{true, false} {
		for _, leaseLost := range []bool{true, false} {
			t.Run(map[bool]string{true: "retry", false: "fail"}[retry]+map[bool]string{true: "/lost", false: "/storage"}[leaseLost], func(t *testing.T) {
				finalError := error(port.ErrLeaseLost)
				if !leaseLost {
					finalError = errors.New("storage unavailable")
				}
				q := &fencedFinalizer{poolQueue: &poolQueue{runs: []port.Run{{ID: "first", Kind: port.RunBacktest, Attempts: 1}, {ID: "unrelated", Kind: port.RunBacktest, Attempts: 1}}}, failure: finalError}
				store := &fencedStore{failure: finalError}
				var subsequent atomic.Bool
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				handler := func(_ context.Context, run port.Run) error {
					if run.ID == "unrelated" {
						subsequent.Store(true)
						cancel()
						return nil
					}
					if retry {
						return port.ErrTemporary
					}
					return ErrIncompleteMarketData
				}
				pool, err := NewWorkerPool(q, store, map[port.RunKind]RunHandler{port.RunBacktest: handler}, WorkerPoolConfig{Owner: "worker", Workers: 1, Lease: time.Hour, PollInterval: time.Millisecond})
				require.NoError(t, err)
				err = pool.Run(ctx)
				if leaseLost {
					require.ErrorIs(t, err, context.Canceled)
					require.True(t, subsequent.Load())
				} else {
					require.ErrorIs(t, err, finalError)
					require.False(t, subsequent.Load())
				}
				if retry {
					require.EqualValues(t, 1, q.writes.Load())
					require.Zero(t, store.writes.Load())
				} else {
					require.EqualValues(t, 1, store.writes.Load())
					require.Zero(t, q.writes.Load())
				}
			})
		}
	}
}
