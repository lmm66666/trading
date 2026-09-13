package application

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/port"
)

// 模拟前查尚未找到任务，但入队时同 hash 的并发胜出者已提交。
type successfulWinnerQueue struct {
	port.JobQueue
	preserveID bool
	change     func(*port.Run)
}

func (q successfulWinnerQueue) FindByIdempotency(context.Context, port.RunKind, string) (port.Run, error) {
	return port.Run{}, port.ErrRunNotFound
}
func (q successfulWinnerQueue) Enqueue(_ context.Context, candidate port.Run) (port.Run, error) {
	if !q.preserveID {
		candidate.ID = "concurrent-winner"
	}
	if q.change != nil {
		q.change(&candidate)
	}
	return candidate, nil
}

func TestEnqueueSuccessStillValidatesCompleteWinnerPayload(t *testing.T) {
	for _, kind := range []port.RunKind{port.RunBacktest, port.RunScan} {
		for _, mode := range []string{"valid", "omitted-zero-field", "old-format", "unknown-field", "candidate-id"} {
			t.Run(string(kind)+"/"+mode, func(t *testing.T) {
				q := successfulWinnerQueue{preserveID: mode == "candidate-id", change: func(run *port.Run) {
					if mode == "valid" {
						return
					}
					if mode == "old-format" {
						run.RequestJSON = []byte(`{}`)
						return
					}
					var payload map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(run.RequestJSON, &payload))
					if mode == "unknown-field" {
						payload["obsolete"] = json.RawMessage(`true`)
					} else {
						key, field := "config", "MinimumCommission"
						if kind == port.RunScan {
							key, field = "scope", "active_only"
						}
						var nested map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(payload[key], &nested))
						delete(nested, field)
						payload[key], _ = json.Marshal(nested)
					}
					run.RequestJSON, _ = json.Marshal(payload)
				}}
				var run port.Run
				var err error
				if kind == port.RunBacktest {
					s, _, _ := newBacktestFixture(t)
					s.queue = q
					run, err = s.Create(context.Background(), computeRequest())
				} else {
					s, _, _, _ := newScanFixture(t, nil)
					s.queue = q
					run, err = s.Create(context.Background(), scanRequest())
				}
				if mode == "valid" {
					require.NoError(t, err)
					require.Equal(t, "concurrent-winner", run.ID)
				} else {
					require.Error(t, err)
					require.Empty(t, run.ID)
				}
			})
		}
	}
}
