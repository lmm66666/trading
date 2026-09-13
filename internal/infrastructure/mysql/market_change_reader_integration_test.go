//go:build integration

package mysql

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
)

func TestApplicationMarketChangeClassificationMySQL(t *testing.T) {
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			ctx := context.Background()
			batch := testBatch(100000)
			first, err := repo.Publish(ctx, batch)
			require.NoError(t, err)
			batch = testBatch(110000)
			second, err := repo.Publish(ctx, batch)
			require.NoError(t, err)
			change, err := repo.MarketChanges(ctx, first, second)
			require.NoError(t, err)
			require.False(t, change.FactorsOrActionsChanged)
			require.Equal(t, []market.InstrumentID{batch.Instrument}, change.Dirty)
			batch.Factors = []market.AdjustmentFactor{{EffectiveTime: batch.Bars[market.Day][0].OpenTime, Numerator: 2, Denominator: 1}}
			third, err := repo.Publish(ctx, batch)
			require.NoError(t, err)
			change, err = repo.MarketChanges(ctx, second, third)
			require.NoError(t, err)
			require.True(t, change.FactorsOrActionsChanged)
			batch.Actions = []market.CorporateAction{{ID: "dividend", Instrument: batch.Instrument, Kind: market.CashDividend, ExDate: batch.Bars[market.Day][0].OpenTime, CashPerShare: 1}}
			fourth, err := repo.Publish(ctx, batch)
			require.NoError(t, err)
			change, err = repo.MarketChanges(ctx, third, fourth)
			require.NoError(t, err)
			require.True(t, change.FactorsOrActionsChanged)
			q := NewJobQueue(db)
			run := queuedRun()
			_, err = q.Enqueue(ctx, run)
			require.NoError(t, err)
			stored, err := q.FindByIdempotency(ctx, port.RunScan, run.IdempotencyKey)
			require.NoError(t, err)
			require.Equal(t, run.ID, stored.ID)
			_, err = q.FindByIdempotency(ctx, port.RunScan, "Key")
			require.ErrorIs(t, err, port.ErrRunNotFound)
		})
	}
}
