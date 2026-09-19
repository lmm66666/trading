//go:build integration

package mysql

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/infrastructure/mysql/dbtest"
)

func TestMigrateCreatesKernelTablesOnMySQL84(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			require.NoError(t, Migrate(db))
			for _, table := range []string{"t_instruments", "t_market_data_versions", "t_market_bars", "t_adjustment_factors", "t_corporate_actions", "t_compute_runs", "t_backtest_runs", "t_backtest_orders", "t_backtest_trades", "t_backtest_equity_points", "t_signal_snapshots", "t_signal_snapshot_rows", "t_outbox_events", "t_watchlist"} {
				require.True(t, db.Migrator().HasTable(table), table)
			}
			var columns []struct{ ColumnName, ColumnType string }
			require.NoError(t, db.Raw("SELECT column_name, column_type FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 't_market_bars' AND column_name IN ('open','high','low','close','amount')").Scan(&columns).Error)
			require.Len(t, columns, 5)
			for _, c := range columns {
				require.Contains(t, c.ColumnType, "bigint")
				require.NotContains(t, c.ColumnType, "unsigned")
			}
		})
	}
}
