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
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
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

func TestDurableSnapshotMySQLPaginationStaysOnSelectedSnapshot(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			seedDurableInstrument(t, db)
			require.NoError(t, db.Create(&InstrumentModel{BaseModel: BaseModel{ID: 42}, Exchange: "SSE", Code: "600001", Source: "test"}).Error)
			ctx := context.Background()
			reader := NewSignalSnapshotStore(db)
			old := testSnapshot()
			old.ID = "Old "
			old.Rows = append(old.Rows, port.SnapshotRow{Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600001"}, SignalTime: old.Key.AsOf, Reason: "old second row"})
			tx := db.Begin()
			require.NoError(t, insertSnapshot(tx, old, port.RunSucceeded, time.Now().UTC()))
			pinned := old.Key
			pinned.SnapshotID = old.ID
			_, err := reader.Latest(ctx, pinned, port.PageRequest{Limit: 1})
			require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			require.NoError(t, tx.Commit().Error)
			first, err := reader.Latest(ctx, old.Key, port.PageRequest{Limit: 1})
			require.NoError(t, err)
			require.Equal(t, old.ID, first.ID)
			require.Equal(t, "600000", first.Rows[0].Instrument.Code)

			newer := testSnapshot()
			newer.ID, newer.RunID, newer.DataVersion = "new", "new run", 2
			newer.Rows[0].Reason = "new first row"
			tx = db.Begin()
			require.NoError(t, insertSnapshot(tx, newer, port.RunSucceeded, time.Now().UTC()))
			require.NoError(t, tx.Commit().Error)
			latest, err := reader.Latest(ctx, old.Key, port.PageRequest{Limit: 1})
			require.NoError(t, err)
			require.Equal(t, newer.ID, latest.ID)
			require.Equal(t, first.Key.AsOf, latest.Key.AsOf)
			second, err := reader.Latest(ctx, first.Key, port.PageRequest{AfterSequence: 1, Limit: 1})
			require.NoError(t, err)
			require.Equal(t, old.ID, second.ID)
			require.Equal(t, "600001", second.Rows[0].Instrument.Code)
			require.Equal(t, "old second row", second.Rows[0].Reason)
			_, err = reader.Latest(ctx, old.Key, port.PageRequest{AfterSequence: 1, Limit: 1})
			require.ErrorIs(t, err, port.ErrInvalidPortValue)

			unpublished := testSnapshot()
			unpublished.ID, unpublished.RunID, unpublished.DataVersion = "pending", "pending run", 3
			tx = db.Begin()
			require.NoError(t, insertSnapshot(tx, unpublished, port.RunPending, time.Now().UTC()))
			require.NoError(t, tx.Commit().Error)
			for _, change := range []func(*port.SnapshotKey){
				func(key *port.SnapshotKey) { key.SnapshotID = "unknown" },
				func(key *port.SnapshotKey) { key.SnapshotID = "old " },
				func(key *port.SnapshotKey) { key.SnapshotID = "Old" },
				func(key *port.SnapshotKey) { key.SnapshotID = "pending" },
				func(key *port.SnapshotKey) { key.StrategyID = "other" },
				func(key *port.SnapshotKey) { key.StrategyVersion = "v2" },
				func(key *port.SnapshotKey) { key.ParametersHash = "other" },
				func(key *port.SnapshotKey) { key.AsOf = key.AsOf.Add(time.Second) },
			} {
				key := pinned
				change(&key)
				_, err := reader.Latest(ctx, key, port.PageRequest{AfterSequence: 1, Limit: 1})
				require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			}
		})
	}
}
