//go:build integration

package mysql

import (
	"context"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
)

func TestRefreshProgressPersistenceAndRecovery(t *testing.T) {
	db := dbtest.OpenIsolatedMySQL(t, dbtest.Target)
	require.NoError(t, Migrate(db))
	store := NewRefreshProgressStore(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	total := 2
	s := port.RefreshSnapshot{Run: port.RefreshRun{RunID: "Run A", Kind: "STOCK", Trigger: "MANUAL", State: "RUNNING", Total: &total, StartedAt: now, HeartbeatAt: now, SnapshotAt: now, Revision: 1}}
	require.NoError(t, store.SaveRefresh(ctx, s))
	s.Run.Revision = 2
	s.Run.Failed = 1
	s.Failures = []port.RefreshFailure{{RunID: "Run A", Exchange: market.SSE, Code: "600000", ErrorCode: "REFRESH_FAILED", CompletedAt: now}}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- store.SaveRefresh(ctx, s) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	got, err := store.GetRefreshRun(ctx, "Run A")
	require.NoError(t, err)
	require.Equal(t, 1, got.Failed)
	f, err := store.ListRefreshFailures(ctx, "Run A", 0, 50)
	require.NoError(t, err)
	require.Len(t, f, 1)
	f, err = store.ListRefreshFailures(ctx, "Run A", f[0].ID, 50)
	require.NoError(t, err)
	require.Empty(t, f)
	_, err = store.GetRefreshRun(ctx, "run a")
	require.ErrorIs(t, err, port.ErrRefreshRunNotFound)
	s.Run.Revision = 1
	s.Run.Failed = 0
	s.Failures = nil
	require.NoError(t, store.SaveRefresh(ctx, s))
	got, _ = store.GetRefreshRun(ctx, "Run A")
	require.Equal(t, 1, got.Failed)
	require.NoError(t, store.InterruptRefreshRunsBefore(ctx, time.Now().UTC()))
	got, err = store.GetRefreshRun(ctx, "Run A")
	require.NoError(t, err)
	require.Equal(t, "INTERRUPTED", got.State)
	s.Run.Revision = 100
	require.NoError(t, store.SaveRefresh(ctx, s))
	got, _ = store.GetRefreshRun(ctx, "Run A")
	require.Equal(t, "INTERRUPTED", got.State)
	rows, err := store.ListRefreshRuns(ctx, "STOCK", 0, 20)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	rows, err = store.ListRefreshRuns(ctx, "STOCK", rows[0].ID, 20)
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = store.ListRefreshRuns(ctx, "INVALID", 0, 20)
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
}
func TestRefreshLatestUsesStartTimeNotDelayedInsertOrder(t *testing.T) {
	db := dbtest.OpenIsolatedMySQL(t, dbtest.Target)
	require.NoError(t, Migrate(db))
	store := NewRefreshProgressStore(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	newer := port.RefreshSnapshot{Run: port.RefreshRun{RunID: "newer", Kind: "STOCK", Trigger: "MANUAL", State: "PREPARING", StartedAt: now, HeartbeatAt: now, SnapshotAt: now, Revision: 1}}
	require.NoError(t, store.SaveRefresh(ctx, newer))
	older := newer
	older.Run.RunID = "older"
	older.Run.StartedAt = now.Add(-time.Hour)
	older.Run.State = "SUCCEEDED"
	older.Run.FinishedAt = &now
	require.NoError(t, store.SaveRefresh(ctx, older))
	latest, err := store.LatestRefreshRun(ctx, "STOCK")
	require.NoError(t, err)
	require.Equal(t, "newer", latest.RunID)
	require.NoError(t, store.InterruptRefreshRunsBefore(ctx, now.Add(-time.Minute)))
	latest, err = store.LatestRefreshRun(ctx, "STOCK")
	require.NoError(t, err)
	require.Equal(t, "PREPARING", latest.State)
}
