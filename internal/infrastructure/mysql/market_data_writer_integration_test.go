//go:build integration

package mysql

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sync"
	"testing"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
)

func TestPublishClosesChangedRevisionAndKeepsUnchangedBar(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			ctx := context.Background()
			b := testBatch(100000)
			v1, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			same, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			require.Equal(t, v1, same)
			b2 := testBatch(110000)
			v2, err := repo.Publish(ctx, b2)
			require.NoError(t, err)
			require.Greater(t, v2, v1)
			var rows []MarketBarModel
			require.NoError(t, db.Order("revision").Find(&rows).Error)
			require.Len(t, rows, 2)
			require.Equal(t, uint64(v2), *rows[0].ValidToVersion)
			require.Nil(t, rows[1].ValidToVersion)
			v3, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			require.Greater(t, v3, v2)
			var count int64
			require.NoError(t, db.Model(&MarketBarModel{}).Count(&count).Error)
			require.Equal(t, int64(3), count)
			b.Digest = "forged"
			_, err = repo.Publish(ctx, b)
			require.Error(t, err)
		})
	}
}

func TestPublishRollsBackRevisionClosuresOnInsertFailure(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			ctx := context.Background()
			v, err := repo.Publish(ctx, testBatch(100000))
			require.NoError(t, err)
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_replacement", func(tx *gorm.DB) {
				if tx.Statement.Table == "t_market_bars" {
					tx.AddError(fmt.Errorf("injected insert failure"))
				}
			}))
			_, err = repo.Publish(ctx, testBatch(110000))
			require.Error(t, err)
			latest, err := repo.LatestCompleteVersion(ctx)
			require.NoError(t, err)
			require.Equal(t, v, latest)
			var bars []MarketBarModel
			require.NoError(t, db.Find(&bars).Error)
			require.Len(t, bars, 1)
			require.Nil(t, bars[0].ValidToVersion)
			var count int64
			require.NoError(t, db.Model(&DataVersionModel{}).Where("version > 0").Count(&count).Error)
			require.Equal(t, int64(1), count)

		})
	}
}

func TestPublishConcurrentSameBatchHasOneVersion(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			versions := make([]market.DataVersion, 8)
			errs := make([]error, 8)
			var wg sync.WaitGroup
			for i := range versions {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					versions[i], errs[i] = repo.Publish(context.Background(), testBatch(100000))
				}(i)
			}
			wg.Wait()
			for i := range versions {
				require.NoError(t, errs[i])
				require.Equal(t, market.DataVersion(1), versions[i])
			}

		})
	}
}
