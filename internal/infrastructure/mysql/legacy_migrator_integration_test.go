//go:build integration

package mysql

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

func TestLegacyMigrationMySQLRestartAndIncompleteVisibility(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			require.NoError(t, db.AutoMigrate(&model.StockInfo{}, &model.StockKlineDaily{}, &model.StockKlineWeekly{}))
			item, source := longLegacyFixture(3)
			info := model.StockInfo{Code: item.ID.Code, Name: item.Name}
			require.NoError(t, db.Create(&info).Error)
			for _, bar := range item.Bars[market.Day] {
				old := model.StockKlineDaily(bar)
				require.NoError(t, db.Create(&old).Error)
			}
			ctx := context.Background()
			opts := MigrationOptions{BatchSize: 1}
			broken := source
			broken.actionErr = errors.New("provider outage")
			incomplete, err := NewLegacyMigrator(db, broken).Run(ctx, opts)
			require.ErrorIs(t, err, ErrMigrationIncomplete)
			require.Equal(t, port.DataIncomplete, incomplete.Quality)
			require.False(t, incomplete.BacktestEnabled)
			var stored DataVersionModel
			require.NoError(t, db.Where("version = ?", incomplete.Version).Take(&stored).Error)
			require.Equal(t, "INCOMPLETE", stored.Status)
			repo := NewMarketDataRepository(db)
			_, err = repo.LatestCompleteVersion(ctx)
			require.Error(t, err)
			_, _, _, err = repo.Dataset(ctx, item.ID, market.Day, incomplete.DailyDates.From, incomplete.DailyDates.To, incomplete.Version)
			require.Error(t, err)
			var count int64
			require.NoError(t, db.Model(&MarketBarModel{}).Count(&count).Error)
			require.Zero(t, count)
			attempts := 0
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_second_legacy_batch", func(tx *gorm.DB) {
				if tx.Statement.Table == "t_market_bars" {
					attempts++
					if attempts == 2 {
						tx.AddError(errors.New("target batch unavailable"))
					}
				}
			}))
			partial, err := NewLegacyMigrator(db, source).Run(ctx, opts)
			require.Error(t, err)
			require.Equal(t, port.DataIncomplete, partial.Quality)
			require.False(t, partial.BacktestEnabled)
			require.NoError(t, db.Model(&MarketBarModel{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
			var progress legacyTargetCursor
			found, err := readMigrationState(db, "target", item.ID.String(), 0, &progress)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, 1, progress.Positions[0])
			_, _, _, err = repo.Dataset(ctx, item.ID, market.Day, partial.DailyDates.From, partial.DailyDates.To, partial.Version)
			require.Error(t, err)
			_, err = repo.Publish(ctx, testBatch(100000))
			require.ErrorIs(t, err, ErrLegacyMigrationBusy)
			require.NoError(t, db.Callback().Create().Remove("fail_second_legacy_batch"))
			// 回填缓存已提交，恢复不依赖可用的 provider，也不重做第一批。
			first, err := NewLegacyMigrator(db, broken).Run(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, incomplete.Version, first.Version)
			require.True(t, first.BacktestEnabled)
			second, err := NewLegacyMigrator(db, broken).Run(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, first, second)
			dataset, _, _, err := repo.Dataset(ctx, item.ID, market.Day, first.DailyDates.From, first.DailyDates.To, first.Version)
			require.NoError(t, err)
			require.Equal(t, 3, dataset.Len())
			require.Equal(t, market.Price(100000), dataset.Bar(0).Close)
			require.NoError(t, db.Model(&MarketBarModel{}).Count(&count).Error)
			require.Equal(t, int64(3), count)
			require.NoError(t, db.Model(&model.StockKlineDaily{}).Count(&count).Error)
			require.Equal(t, int64(3), count)
		})
	}
}
