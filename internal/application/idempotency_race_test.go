package application

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
	"trading/internal/port"
)

type submissionOrderKey struct{}
type racingQueue struct {
	port.JobQueue
	mu           sync.Mutex
	run          port.Run
	found        atomic.Int64
	bothChecked  chan struct{}
	winnerStored chan struct{}
}

func newRacingQueue() *racingQueue {
	return &racingQueue{bothChecked: make(chan struct{}), winnerStored: make(chan struct{})}
}
func (q *racingQueue) FindByIdempotency(ctx context.Context, _ port.RunKind, _ string) (port.Run, error) {
	n := q.found.Add(1)
	if n <= 2 {
		if n == 2 {
			close(q.bothChecked)
		}
		select {
		case <-ctx.Done():
			return port.Run{}, ctx.Err()
		case <-q.bothChecked:
			return port.Run{}, port.ErrRunNotFound
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.run, nil
}
func (q *racingQueue) Enqueue(ctx context.Context, run port.Run) (port.Run, error) {
	if ctx.Value(submissionOrderKey{}) == 2 {
		select {
		case <-ctx.Done():
			return port.Run{}, ctx.Err()
		case <-q.winnerStored:
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.run.ID == "" {
		q.run = run
		close(q.winnerStored)
		return run, nil
	}
	if q.run.InputHash != run.InputHash {
		return port.Run{}, port.ErrIdempotencyConflict
	}
	return q.run, nil
}

type racingMarket struct {
	port.MarketData
	versionChanges bool
}

func (m racingMarket) LatestCompleteVersion(ctx context.Context) (market.DataVersion, error) {
	if m.versionChanges && ctx.Value(submissionOrderKey{}) == 2 {
		return 8, nil
	}
	return 7, nil
}
func (m racingMarket) Instruments(context.Context, port.InstrumentScope) ([]market.InstrumentID, error) {
	return []market.InstrumentID{marketID}, nil
}

type racingSnapshots struct{}

func (racingSnapshots) Latest(ctx context.Context, key port.SnapshotKey, _ port.PageRequest) (port.SignalSnapshot, error) {
	id := "old"
	if ctx.Value(submissionOrderKey{}) == 2 {
		id = "new"
	}
	return port.SignalSnapshot{ID: id, Key: key, DataVersion: 7}, nil
}

func TestConcurrentIdempotencyUsesWinnerPinnedInputs(t *testing.T) {
	for _, kind := range []port.RunKind{port.RunBacktest, port.RunScan} {
		for _, changed := range []bool{false, true} {
			for _, conflict := range []bool{false, true} {
				t.Run(string(kind)+map[bool]string{false: "/snapshot", true: "/version"}[changed]+map[bool]string{false: "/same", true: "/conflict"}[conflict], func(t *testing.T) {
					q := newRacingQueue()
					registry := computeRegistry(t)
					store := &computeStore{}
					data := racingMarket{versionChanges: changed}
					backtests, err := NewBacktestService(registry, failedEngine{}, data, q, store, ComputeConfig{EngineVersion: "1"})
					require.NoError(t, err)
					scans, err := NewScanService(registry, data, q, store, racingSnapshots{}, ScanConfig{ComputeConfig: ComputeConfig{EngineVersion: "1"}, Workers: 1})
					require.NoError(t, err)
					type outcome struct {
						run port.Run
						err error
					}
					results := make([]outcome, 2)
					var wg sync.WaitGroup
					for i := 0; i < 2; i++ {
						wg.Add(1)
						go func(i int) {
							defer wg.Done()
							ctx := context.WithValue(context.Background(), submissionOrderKey{}, i+1)
							if kind == port.RunBacktest {
								local := *backtests
								local.config.EngineVersion = []string{"1", "2"}[i]
								r := computeRequest()
								if conflict && i == 1 {
									r.Config.SlippageBPS++
								}
								results[i].run, results[i].err = local.Create(ctx, r)
							} else {
								local := *scans
								local.config.EngineVersion = []string{"1", "2"}[i]
								r := scanRequest()
								if conflict && i == 1 {
									r.Scope.ActiveOnly = true
								}
								results[i].run, results[i].err = local.Create(ctx, r)
							}
						}(i)
					}
					wg.Wait()
					require.NoError(t, results[0].err)
					if conflict {
						require.ErrorIs(t, results[1].err, port.ErrIdempotencyConflict)
						return
					}
					require.NoError(t, results[1].err)
					require.Equal(t, results[0].run.ID, results[1].run.ID)
					require.Equal(t, market.DataVersion(7), results[1].run.DataVersion)
					require.Equal(t, "1", results[1].run.EngineVersion)
					if kind == port.RunScan {
						var saved scanInput
						require.NoError(t, json.Unmarshal(results[1].run.RequestJSON, &saved))
						require.Equal(t, "old", saved.PreviousSnapshotID)
					}
				})
			}
		}
	}
}
