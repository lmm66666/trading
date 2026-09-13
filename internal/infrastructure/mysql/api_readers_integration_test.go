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

func TestAPIReadersMySQLPublishedOrderingAndActiveIdentity(t *testing.T) {
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
			require.NoError(t, Migrate(db))
			ctx := context.Background()
			instruments := []InstrumentModel{{Exchange: "SSE", Code: "600000", Active: true, Source: "test"}, {Exchange: "BSE", Code: "600000", Active: true, Source: "test"}, {Exchange: "SZSE", Code: "600000", Active: false, Source: "test"}, {Exchange: "SZSE", Code: "000001", Active: true, Source: "test"}}
			require.NoError(t, db.Create(&instruments).Error)
			marketData := NewMarketDataRepository(db)
			ids, err := marketData.ResolveCode(ctx, "600000")
			require.NoError(t, err)
			require.Len(t, ids, 2)
			require.Equal(t, "BSE:600000", ids[0].String())
			require.Equal(t, "SSE:600000", ids[1].String())
			ids, err = marketData.ResolveCode(ctx, "000001")
			require.NoError(t, err)
			require.Len(t, ids, 1)
			ids, err = marketData.ResolveCode(ctx, "999999")
			require.NoError(t, err)
			require.Empty(t, ids)
			now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			snapshots := []SignalSnapshotModel{
				{SnapshotID: "old", RunID: "old-run", StrategyID: "strategy", StrategyVersion: "1", ParametersHash: "first", DataVersion: 1, AsOf: now, Status: "SUCCEEDED", FailuresJSON: []byte(`[]`)},
				{SnapshotID: "same-time-higher-version", RunID: "second-run", StrategyID: "strategy", StrategyVersion: "2", ParametersHash: "second", DataVersion: 2, AsOf: now, Status: "SUCCEEDED", FailuresJSON: []byte(`[]`)},
				{SnapshotID: "Latest ", RunID: "third-run", StrategyID: "strategy", StrategyVersion: "3", ParametersHash: "Third ", DataVersion: 2, AsOf: now, Status: "PARTIAL_SUCCEEDED", FailuresJSON: []byte(`[]`)},
				{SnapshotID: "pending", RunID: "pending-run", StrategyID: "strategy", StrategyVersion: "3", ParametersHash: "third", DataVersion: 3, AsOf: now.Add(time.Hour), Status: "PENDING", FailuresJSON: []byte(`[]`)},
			}
			for i := range snapshots {
				require.NoError(t, db.Create(&snapshots[i]).Error)
			}
			reader := NewSignalSnapshotStore(db)
			key, err := reader.LatestPublishedKey(ctx, "strategy", "", "")
			require.NoError(t, err)
			require.Equal(t, "Latest ", key.SnapshotID)
			require.Equal(t, "Third ", key.ParametersHash)
			key, err = reader.LatestPublishedKey(ctx, "strategy", "1", "")
			require.NoError(t, err)
			require.Equal(t, "old", key.SnapshotID)
			for _, id := range []string{"Latest", "latest ", "pending", "unknown"} {
				_, err = reader.LatestPublishedKey(ctx, "strategy", "", id)
				require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			}
			_, err = reader.LatestPublishedKey(ctx, "Strategy", "", "")
			require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			key, err = reader.LatestPublishedKey(ctx, "strategy", "", "Latest ")
			require.NoError(t, err)
			require.Equal(t, "Latest ", key.SnapshotID)
		})
	}
}
