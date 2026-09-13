//go:build integration

package mysql

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

func TestLegacyMigrationMySQLRestartAndIncompleteVisibility(t *testing.T) {
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
			require.NoError(t, Migrate(db))
			require.NoError(t, db.AutoMigrate(&model.StockInfo{}, &model.StockKlineDaily{}, &model.StockKlineWeekly{}))
			item, source := legacyFixture(t)
			info := model.StockInfo{Code: item.ID.Code, Name: item.Name}
			require.NoError(t, db.Create(&info).Error)
			old := model.StockKlineDaily(item.Bars[market.Day][0])
			require.NoError(t, db.Create(&old).Error)
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
			first, err := NewLegacyMigrator(db, source).Run(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, incomplete.Version, first.Version)
			require.True(t, first.BacktestEnabled)
			second, err := NewLegacyMigrator(db, broken).Run(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, first, second)
			dataset, _, _, err := repo.Dataset(ctx, item.ID, market.Day, first.DailyDates.From, first.DailyDates.To, first.Version)
			require.NoError(t, err)
			require.Equal(t, 1, dataset.Len())
			require.Equal(t, market.Price(100000), dataset.Bar(0).Close)
			require.NoError(t, db.Model(&MarketBarModel{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
			require.NoError(t, db.Model(&model.StockKlineDaily{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}
