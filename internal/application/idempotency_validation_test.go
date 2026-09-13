package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/market"
	"trading/internal/port"
)

func TestIdempotentReuseRejectsIncompleteOrChangedSavedPayload(t *testing.T) {
	for _, kind := range []port.RunKind{port.RunBacktest, port.RunScan} {
		for _, change := range []string{"missing", "unknown", "trailing", "hash", "version", "identity"} {
			t.Run(string(kind)+"/"+change, func(t *testing.T) {
				var run port.Run
				var err error
				s, _, _ := newBacktestFixture(t)
				sc, _, _, _ := newScanFixture(t, []market.InstrumentID{marketID})
				if kind == port.RunBacktest {
					run, err = s.Create(context.Background(), computeRequest())
				} else {
					run, err = sc.Create(context.Background(), scanRequest())
				}
				require.NoError(t, err)
				switch change {
				case "missing":
					var payload map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(run.RequestJSON, &payload))
					if kind == port.RunBacktest {
						var config map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(payload["config"], &config))
						delete(config, "MinimumCommission")
						payload["config"], _ = json.Marshal(config)
					} else {
						var scope map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(payload["scope"], &scope))
						delete(scope, "active_only")
						payload["scope"], _ = json.Marshal(scope)
					}
					run.RequestJSON, _ = json.Marshal(payload)
				case "unknown":
					var payload map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(run.RequestJSON, &payload))
					payload["unknown"] = json.RawMessage(`true`)
					run.RequestJSON, _ = json.Marshal(payload)
				case "trailing":
					run.RequestJSON = append(run.RequestJSON, []byte(` {}`)...)
				case "hash":
					run.InputHash = "changed"
				case "version":
					run.DataVersion = 0
				case "identity":
					run.IdempotencyKey = "another"
				}
				if kind == port.RunBacktest {
					request := computeRequest()
					_, request.Parameters, err = resolveComputeStrategy(s.registry, request.StrategyID, request.StrategyVersion, request.Parameters)
					require.NoError(t, err)
					_, err = reuseBacktestSubmission(s.registry, request, run)
				} else {
					request := scanRequest()
					_, request.Parameters, err = resolveComputeStrategy(sc.registry, request.StrategyID, request.StrategyVersion, request.Parameters)
					require.NoError(t, err)
					_, err = reuseScanSubmission(sc.registry, request, run)
				}
				require.Error(t, err)
			})
		}
	}
}

type lookupFailureQueue struct {
	port.JobQueue
	run port.Run
	err error
}

func (q lookupFailureQueue) FindByIdempotency(context.Context, port.RunKind, string) (port.Run, error) {
	return q.run, q.err
}
func TestIdempotentCollisionDoesNotSwallowOtherStorageErrors(t *testing.T) {
	ctx := context.Background()
	unavailable := errors.New("unavailable")
	for _, test := range []struct {
		queue            port.JobQueue
		enqueueErr, want error
	}{{lookupFailureQueue{}, unavailable, unavailable}, {lookupFailureQueue{err: unavailable}, port.ErrIdempotencyConflict, unavailable}, {lookupFailureQueue{err: port.ErrRunNotFound}, port.ErrIdempotencyConflict, port.ErrIdempotencyConflict}, {&poolQueue{}, port.ErrIdempotencyConflict, port.ErrIdempotencyConflict}} {
		_, found, err := collidedSubmission(ctx, test.queue, port.RunBacktest, "key", test.enqueueErr)
		require.False(t, found)
		require.ErrorIs(t, err, test.want)
	}
}

func TestScanReuseComparesAllUserIntentFields(t *testing.T) {
	s, _, store, _ := newScanFixture(t, []market.InstrumentID{marketID})
	_, err := s.Create(context.Background(), scanRequest())
	require.NoError(t, err)
	for _, edit := range []func(*ScanRequest){func(r *ScanRequest) { r.From = marketDate(2) }, func(r *ScanRequest) { r.AsOf = marketDate(4) }, func(r *ScanRequest) { r.Scope.ActiveOnly = true }, func(r *ScanRequest) { r.Parameters["a"] = 2 }} {
		request := scanRequest()
		_, request.Parameters, err = resolveComputeStrategy(s.registry, request.StrategyID, request.StrategyVersion, nil)
		require.NoError(t, err)
		edit(&request)
		_, err = reuseScanSubmission(s.registry, request, store.run)
		require.ErrorIs(t, err, port.ErrIdempotencyConflict)
	}
}
