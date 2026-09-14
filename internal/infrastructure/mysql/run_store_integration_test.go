//go:build integration

package mysql

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/port"
)

func TestDurableBacktestMySQLBatchesRollbackAndPagination(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			seedDurableInstrument(t, db)
			ctx := context.Background()
			queue := NewJobQueue(db)
			store := NewRunStore(db)
			input := queuedRun()
			input.Kind = port.RunBacktest
			input.RequestJSON = []byte(`{"instrument":{"Exchange":"SSE","Code":"600000"}}`)
			_, err := queue.Enqueue(ctx, input)
			require.NoError(t, err)
			run, err := queue.Claim(ctx, "worker", time.Minute)
			require.NoError(t, err)
			calls := 0
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_second_equity_batch", func(tx *gorm.DB) {
				if tx.Statement.Table == "t_backtest_equity_points" {
					calls++
					if calls == 2 {
						tx.AddError(errors.New("injected failure"))
					}
				}
			}))
			result := testBacktestResult(1001)
			require.Error(t, store.CompleteBacktest(ctx, run.ID, run.LeaseToken, result))
			require.NoError(t, db.Callback().Create().Remove("test:fail_second_equity_batch"))
			for _, model := range []any{&BacktestRunModel{}, &BacktestOrderModel{}, &BacktestTradeModel{}, &BacktestEquityModel{}, &OutboxEventModel{}} {
				var count int64
				require.NoError(t, db.Model(model).Count(&count).Error)
				require.Zero(t, count)
			}
			got, err := store.Get(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, port.RunRunning, got.Status)
			require.NoError(t, store.CompleteBacktest(ctx, run.ID, run.LeaseToken, result))
			orders, err := store.Orders(ctx, run.ID, port.PageRequest{Limit: 1000})
			require.NoError(t, err)
			require.Len(t, orders.Items, 1000)
			require.EqualValues(t, 1000, *orders.NextSequence)
			last, err := store.Orders(ctx, run.ID, port.PageRequest{AfterSequence: *orders.NextSequence, Limit: 1000})
			require.NoError(t, err)
			require.Len(t, last.Items, 1)
			require.Nil(t, last.NextSequence)
			require.Equal(t, "Order 1000 ", last.Items[0].ID)
			fills, err := store.Trades(ctx, run.ID, port.PageRequest{AfterSequence: 1000, Limit: 1000})
			require.NoError(t, err)
			require.Equal(t, result.Fills[1000].Gross, fills.Items[0].Gross)
			equity, err := store.Equity(ctx, run.ID, port.PageRequest{AfterSequence: 1000, Limit: 1000})
			require.NoError(t, err)
			require.Equal(t, result.Equity[1000], equity.Items[0])
			summary, err := store.BacktestResult(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, result.Summary, summary)
			var count int64
			require.NoError(t, db.Model(&OutboxEventModel{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
