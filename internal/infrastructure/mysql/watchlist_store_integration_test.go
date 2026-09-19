//go:build integration

package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
)

func TestWatchlistStoreCRUDOrderingAndActiveFilter(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			ctx := context.Background()
			pfu := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
			citic := market.InstrumentID{Exchange: market.SZSE, Code: "000001"}
			stopped := market.InstrumentID{Exchange: market.BSE, Code: "920000"}
			instruments := []InstrumentModel{
				{Exchange: string(pfu.Exchange), Code: pfu.Code, Name: "浦发银行", Board: "MAIN", Active: true, LotSize: 100, Source: "fixture"},
				{Exchange: string(citic.Exchange), Code: citic.Code, Name: "平安银行", Board: "MAIN", Active: true, LotSize: 100, Source: "fixture"},
				{Exchange: string(stopped.Exchange), Code: stopped.Code, Name: "停用证券", Board: "MAIN", Active: false, LotSize: 100, Source: "fixture"},
			}
			for i := range instruments {
				require.NoError(t, db.Create(&instruments[i]).Error)
			}
			store := NewWatchlistStore(db)

			exists, err := store.Add(ctx, pfu)
			require.NoError(t, err)
			require.False(t, exists)
			exists, err = store.Add(ctx, pfu)
			require.NoError(t, err)
			require.True(t, exists)
			exists, err = store.Add(ctx, citic)
			require.NoError(t, err)
			require.False(t, exists)
			count, err := store.Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 2, count)

			_, err = store.Add(ctx, stopped)
			require.ErrorIs(t, err, port.ErrMarketDataNotFound)
			_, err = store.Add(ctx, market.InstrumentID{Exchange: market.SSE, Code: "999999"})
			require.ErrorIs(t, err, port.ErrMarketDataNotFound)

			entries, err := store.List(ctx)
			require.NoError(t, err)
			require.Len(t, entries, 2)
			require.Equal(t, pfu, entries[0].ID)
			require.Equal(t, "浦发银行", entries[0].Name)
			require.Equal(t, "MAIN", entries[0].Board)
			require.Equal(t, int64(100), entries[0].LotSize)
			require.Equal(t, instruments[0].ID, entries[0].InstrumentRowID)
			require.Equal(t, citic, entries[1].ID)

			require.NoError(t, store.Remove(ctx, pfu))
			count, err = store.Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, count)
			require.NoError(t, store.Remove(ctx, pfu))
			count, err = store.Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, count)

			entries, err = store.List(ctx)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, citic, entries[0].ID)

			exists, err = store.Add(ctx, pfu)
			require.NoError(t, err)
			require.False(t, exists)
			entries, err = store.List(ctx)
			require.NoError(t, err)
			require.Len(t, entries, 2)
			require.Equal(t, citic, entries[0].ID)
			require.Equal(t, pfu, entries[1].ID)
		})
	}
}

func TestWatchlistListFiltersInactiveStoredRows(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			ctx := context.Background()
			delisted := market.InstrumentID{Exchange: market.SSE, Code: "600001"}
			require.NoError(t, db.Create(&InstrumentModel{Exchange: "SSE", Code: "600001", Name: "退市证券", Board: "MAIN", Active: false, LotSize: 100, Source: "fixture"}).Error)
			require.NoError(t, db.Create(&WatchlistModel{Exchange: string(delisted.Exchange), Code: delisted.Code}).Error)
			store := NewWatchlistStore(db)

			entries, err := store.List(ctx)
			require.NoError(t, err)
			require.Empty(t, entries)
			count, err := store.Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, count)
		})
	}
}

func TestWatchlistLatestDailyQuotes(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			ctx := context.Background()
			require.NoError(t, db.Create(&DataVersionModel{Version: 1, Source: "fixture", Status: versionComplete, Quality: "COMPLETE"}).Error)
			day := time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)
			twoBars := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
			oneBar := market.InstrumentID{Exchange: market.SZSE, Code: "000001"}
			noBars := market.InstrumentID{Exchange: market.BSE, Code: "920000"}
			instruments := []InstrumentModel{
				{Exchange: string(twoBars.Exchange), Code: twoBars.Code, Name: "两根", Board: "MAIN", Active: true, LotSize: 100, Source: "fixture"},
				{Exchange: string(oneBar.Exchange), Code: oneBar.Code, Name: "一根", Board: "MAIN", Active: true, LotSize: 100, Source: "fixture"},
				{Exchange: string(noBars.Exchange), Code: noBars.Code, Name: "零根", Board: "MAIN", Active: true, LotSize: 100, Source: "fixture"},
			}
			for i := range instruments {
				require.NoError(t, db.Create(&instruments[i]).Error)
			}
			bars := []MarketBarModel{
				{InstrumentID: instruments[0].ID, Timeframe: "DAY", OpenTime: day.Add(-48 * time.Hour), CloseTime: day.Add(-24 * time.Hour), Revision: 1, ValidFromVersion: 1, Open: 100000, High: 100000, Low: 100000, Close: 100000, Volume: 1, Amount: 100000, TradingStatus: "TRADABLE"},
				{InstrumentID: instruments[0].ID, Timeframe: "DAY", OpenTime: day.Add(-24 * time.Hour), CloseTime: day, Revision: 1, ValidFromVersion: 1, Open: 123400, High: 123400, Low: 123400, Close: 123400, Volume: 2, Amount: 123400, TradingStatus: "TRADABLE"},
				{InstrumentID: instruments[1].ID, Timeframe: "DAY", OpenTime: day.Add(-24 * time.Hour), CloseTime: day, Revision: 1, ValidFromVersion: 1, Open: 123400, High: 123400, Low: 123400, Close: 123400, Volume: 1, Amount: 123400, TradingStatus: "TRADABLE"},
				{InstrumentID: instruments[0].ID, Timeframe: "WEEK", OpenTime: day.Add(-24 * time.Hour), CloseTime: day, Revision: 1, ValidFromVersion: 1, Open: 999999, High: 999999, Low: 999999, Close: 999999, Volume: 1, Amount: 999999, TradingStatus: "TRADABLE"},
			}
			for i := range bars {
				require.NoError(t, db.Create(&bars[i]).Error)
			}
			expired := uint64(2)
			require.NoError(t, db.Create(&MarketBarModel{InstrumentID: instruments[0].ID, Timeframe: "DAY", OpenTime: day, CloseTime: day.Add(time.Hour), Revision: 1, ValidFromVersion: 1, ValidToVersion: &expired, Open: 999999, High: 999999, Low: 999999, Close: 999999, Volume: 1, Amount: 999999, TradingStatus: "TRADABLE"}).Error)
			require.NoError(t, db.Create(&DataVersionModel{Version: 3, Source: "fixture", Status: versionPending, Quality: "COMPLETE"}).Error)
			require.NoError(t, db.Create(&MarketBarModel{InstrumentID: instruments[0].ID, Timeframe: "DAY", OpenTime: day, CloseTime: day.Add(2 * time.Hour), Revision: 2, ValidFromVersion: 3, Open: 999999, High: 999999, Low: 999999, Close: 999999, Volume: 1, Amount: 999999, TradingStatus: "TRADABLE"}).Error)
			store := NewWatchlistStore(db)

			quotes, err := store.LatestDailyQuotes(ctx, []uint64{instruments[0].ID, instruments[1].ID, instruments[2].ID})
			require.NoError(t, err)
			two, ok := quotes[instruments[0].ID]
			require.True(t, ok)
			require.NotNil(t, two.Close)
			require.InDelta(t, 12.34, *two.Close, 1e-9)
			require.NotNil(t, two.Change)
			require.InDelta(t, 2.34, *two.Change, 1e-9)
			require.NotNil(t, two.ChangePercent)
			require.InDelta(t, 23.4, *two.ChangePercent, 1e-9)
			one, ok := quotes[instruments[1].ID]
			require.True(t, ok)
			require.NotNil(t, one.Close)
			require.InDelta(t, 12.34, *one.Close, 1e-9)
			require.Nil(t, one.Change)
			require.Nil(t, one.ChangePercent)
			require.NotContains(t, quotes, instruments[2].ID)

			quotes, err = store.LatestDailyQuotes(ctx, nil)
			require.NoError(t, err)
			require.Empty(t, quotes)
		})
	}
}

func TestWatchlistLatestDailyQuotesWithoutCompleteVersion(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			instrument := InstrumentModel{Exchange: "SSE", Code: "600000", Name: "浦发银行", Board: "MAIN", Active: true, LotSize: 100, Source: "fixture"}
			require.NoError(t, db.Create(&instrument).Error)
			store := NewWatchlistStore(db)

			quotes, err := store.LatestDailyQuotes(context.Background(), []uint64{instrument.ID})
			require.NoError(t, err)
			require.Empty(t, quotes)
		})
	}
}
