//go:build integration

package mysql

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
)

func TestDurableSnapshotMySQLAtomicVisibility(t *testing.T) {
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
			require.NoError(t, Migrate(db))
			seedDurableInstrument(t, db)
			ctx := context.Background()
			snap := testSnapshot()
			reader := NewSignalSnapshotStore(db)
			tx := db.Begin()
			require.NoError(t, tx.Error)
			require.NoError(t, insertSnapshot(tx, snap, port.RunSucceeded, time.Now().UTC()))
			_, err := reader.Latest(ctx, snap.Key, port.PageRequest{Limit: 1000})
			require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			require.NoError(t, tx.Rollback().Error)
			queue := NewJobQueue(db)
			_, err = queue.Enqueue(ctx, queuedRun())
			require.NoError(t, err)
			run, err := queue.Claim(ctx, "worker", time.Minute)
			require.NoError(t, err)
			snap.Failures = map[market.InstrumentID]port.Failure{{Exchange: market.SSE, Code: "600001"}: {Code: "DATA", Message: "missing"}}
			require.NoError(t, NewRunStore(db).CompleteScan(ctx, run.ID, run.LeaseToken, snap))
			published, err := reader.Latest(ctx, snap.Key, port.PageRequest{Limit: 1000})
			require.NoError(t, err)
			require.Equal(t, snap.Rows, published.Rows)
			require.Equal(t, snap.Failures, published.Failures)
			got, err := NewRunStore(db).Get(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, port.RunPartialSucceeded, got.Status)
			var count int64
			require.NoError(t, db.Model(&OutboxEventModel{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			require.ErrorIs(t, NewRunStore(db).CompleteScan(ctx, run.ID, run.LeaseToken, snap), port.ErrLeaseLost)
		})
	}
}
