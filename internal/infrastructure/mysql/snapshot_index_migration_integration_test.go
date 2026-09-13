//go:build integration

package mysql

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/port"
)

func TestApplicationSnapshotIndexUpgradeAndRepeatedRunsMySQL(t *testing.T) {
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
			require.NoError(t, Migrate(db))
			require.NoError(t, db.Exec("ALTER TABLE t_signal_snapshots ADD UNIQUE INDEX uq_snapshot_business (strategy_id, strategy_version, parameters_hash, data_version, as_of)").Error)
			require.True(t, db.Migrator().HasIndex(&SignalSnapshotModel{}, "uq_snapshot_business"))
			require.NoError(t, Migrate(db))
			require.NoError(t, Migrate(db))
			require.False(t, db.Migrator().HasIndex(&SignalSnapshotModel{}, "uq_snapshot_business"))
			for _, name := range []string{"uq_snapshot_id", "uq_snapshot_run", "idx_snapshot_latest"} {
				require.True(t, db.Migrator().HasIndex(&SignalSnapshotModel{}, name))
			}
			seedDurableInstrument(t, db)
			ctx := context.Background()
			queue := NewJobQueue(db)
			store := NewRunStore(db)
			reader := NewSignalSnapshotStore(db)
			var first port.SignalSnapshot
			for _, id := range []string{"first", "second"} {
				run := queuedRun()
				run.ID = id
				run.IdempotencyKey = id
				_, err := queue.Enqueue(ctx, run)
				require.NoError(t, err)
				claimed, err := queue.Claim(ctx, "worker", time.Minute)
				require.NoError(t, err)
				snapshot := testSnapshot()
				snapshot.ID = id
				snapshot.RunID = run.ID
				snapshot.Rows[0].Reason = id
				require.NoError(t, store.CompleteScan(ctx, run.ID, claimed.LeaseToken, snapshot))
				require.ErrorIs(t, store.CompleteScan(ctx, run.ID, claimed.LeaseToken, snapshot), port.ErrLeaseLost)
				published, err := reader.Latest(ctx, snapshot.Key, port.PageRequest{Limit: 1000})
				require.NoError(t, err)
				require.Equal(t, id, published.ID)
				if id == "first" {
					first = published
				}
			}
			pinned, err := reader.Latest(ctx, first.Key, port.PageRequest{Limit: 1000})
			require.NoError(t, err)
			require.Equal(t, "first", pinned.Rows[0].Reason)
			var count int64
			require.NoError(t, db.Model(&SignalSnapshotModel{}).Count(&count).Error)
			require.EqualValues(t, 2, count)
		})
	}
}
