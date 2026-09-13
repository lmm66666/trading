package port_test

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestJobQueueExposesExpiredLeaseRecovery(t *testing.T) {
	var queue port.JobQueue = &fakeJobQueue{}
	count, err := queue.ReapExpired(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestRunAttemptsRejectsInvalidPersistedCounts(t *testing.T) {
	run := port.Run{ID: "run", IdempotencyKey: "key", InputHash: "hash", Kind: port.RunScan, Status: port.RunPending, StrategyID: "s", StrategyVersion: "v", EngineVersion: "v", DataVersion: 1, RequestJSON: []byte(`{}`)}
	for _, attempts := range []int{-1, 5} {
		run.Attempts = attempts
		require.ErrorIs(t, run.Validate(), port.ErrInvalidPortValue)
	}
	for attempts := 0; attempts <= 4; attempts++ {
		run.Attempts = attempts
		require.NoError(t, run.Validate())
	}
}
