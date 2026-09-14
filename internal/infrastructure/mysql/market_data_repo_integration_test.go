//go:build integration

package mysql

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/market"
	"trading/internal/port"
)

func requestFor(b port.MarketWriteBatch, v market.DataVersion) port.BatchRequest {
	return port.BatchRequest{PrimaryTimeframe: market.Day, From: b.Bars[market.Day][0].OpenTime, To: b.Bars[market.Day][0].CloseTime, Version: v}
}

func TestDatasetReadsRevisionVisibleAtRequestedVersion(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			ctx := context.Background()
			b := testBatch(100000)
			v1, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			b2 := testBatch(110000)
			v2, err := repo.Publish(ctx, b2)
			require.NoError(t, err)
			for v, want := range map[market.DataVersion]market.Price{v1: 100000, v2: 110000} {
				req := requestFor(b, v)
				d, _, _, err := repo.Dataset(ctx, b.Instrument, market.Day, req.From, req.To, v)
				require.NoError(t, err)
				require.Equal(t, want, d.Bar(0).Close)
			}
			require.NoError(t, db.Create(&DataVersionModel{Version: 99, Source: "fixture", Status: versionPending, Quality: "COMPLETE"}).Error)
			latest, err := repo.LatestCompleteVersion(ctx)
			require.NoError(t, err)
			require.Equal(t, v2, latest)
			req := requestFor(b, 99)
			_, _, _, err = repo.Dataset(ctx, b.Instrument, market.Day, req.From, req.To, 99)
			require.Error(t, err)
		})
	}
}

func TestBatchDatasetsUsesBoundedStatementCount(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			ctx := context.Background()
			b := testBatch(100000)
			v, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			ids := make([]market.InstrumentID, 5000)
			rows := make([]InstrumentModel, 5000)
			for i := range ids {
				ids[i] = market.InstrumentID{Exchange: market.SZSE, Code: fmt.Sprintf("%06d", i)}
				rows[i] = InstrumentModel{Exchange: "SZSE", Code: ids[i].Code, Source: "fixture"}
			}
			require.NoError(t, db.CreateInBatches(rows, 500).Error)
			// Three representatives across the 5000-ID set each have three
			// timeframes, five warmup bars, two window bars, a future bar and
			// corrections to both a warmup and a window bar.
			from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			representatives := []int{0, 2499, 4999}
			for _, index := range representatives {
				var instrument InstrumentModel
				require.NoError(t, db.Where("exchange = ? AND code = ?", "SZSE", ids[index].Code).Take(&instrument).Error)
				var history []MarketBarModel
				for _, tf := range []market.Timeframe{market.Day, market.Week, market.Month} {
					for _, day := range []int{-5, -4, -3, -2, -1, 0, 1, 3} {
						price := market.Price(100000 + index + int(tf)*100)
						bar := market.Bar{Instrument: ids[index], Timeframe: tf, OpenTime: from.AddDate(0, 0, day).Add(time.Hour), CloseTime: from.AddDate(0, 0, day).Add(7 * time.Hour), Open: price, High: price + 1000, Low: price - 1000, Close: price, Volume: 100}
						old := barModel(instrument.ID, bar, 1, uint64(v))
						if day == -1 || day == 0 {
							next := uint64(v + 1)
							old.ValidToVersion = &next
							bar.Close += 500
							history = append(history, barModel(instrument.ID, bar, 2, next))
						}
						history = append(history, old)
					}
				}
				require.NoError(t, db.Create(&history).Error)
			}
			require.NoError(t, db.Create(&DataVersionModel{Version: uint64(v + 1), Source: "fixture", Status: versionComplete, Quality: "COMPLETE"}).Error)
			var selects atomic.Int64
			repo = NewMarketDataRepository(db.Session(&gorm.Session{Logger: selectCounter{Interface: db.Logger, count: &selects}}))
			for _, version := range []market.DataVersion{v, v + 1} {
				selects.Store(0)
				req := port.BatchRequest{PrimaryTimeframe: market.Day, Auxiliary: []market.Timeframe{market.Week, market.Month}, From: from, To: from.AddDate(0, 0, 2), LookbackBars: 2, Version: version}
				bundles, errs := repo.BatchDatasets(ctx, ids, req)
				require.Empty(t, errs)
				require.Len(t, bundles, 5000)
				require.LessOrEqual(t, selects.Load(), int64(8))
				for _, index := range representatives {
					bundle := bundles[ids[index]]
					for _, tf := range []market.Timeframe{market.Day, market.Week, market.Month} {
						dataset := bundle.Primary
						if tf != market.Day {
							dataset = bundle.Auxiliary[tf]
						}
						require.Equal(t, 4, dataset.Len())
						require.Equal(t, ids[index], dataset.Instrument())
						require.Equal(t, tf, dataset.Timeframe())
						for i, day := range []int{-2, -1, 0, 1} {
							bar := dataset.Bar(i)
							want := market.Price(100000 + index + int(tf)*100)
							if version == v+1 && (day == -1 || day == 0) {
								want += 500
							}
							require.Equal(t, from.AddDate(0, 0, day).Add(7*time.Hour), bar.CloseTime)
							require.Equal(t, want, bar.Close)
							require.Equal(t, version, bar.Version)
						}
					}
				}
				require.Zero(t, bundles[ids[1]].Primary.Len())
			}
		})
	}
}

type selectCounter struct {
	logger.Interface
	count *atomic.Int64
}

func (c selectCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	query, rows := fc()
	text := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(text, "SELECT") || strings.HasPrefix(text, "(SELECT") {
		c.count.Add(1)
	}
	c.Interface.Trace(ctx, begin, func() (string, int64) { return query, rows }, err)
}

func TestDirtyInstrumentsIncludesFactorAndActionRevisions(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			repo := NewMarketDataRepository(db)
			ctx := context.Background()
			b := testBatch(100000)
			v1, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			b.Bars = nil
			b.Factors = []market.AdjustmentFactor{{EffectiveTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Numerator: 1, Denominator: 1}}
			v2, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			ids, err := repo.DirtyInstruments(ctx, v1, v2)
			require.NoError(t, err)
			require.Equal(t, []market.InstrumentID{b.Instrument}, ids)
			b.Factors = nil
			b.Actions = []market.CorporateAction{{ID: "dividend-1", Instrument: b.Instrument, ExDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Kind: market.CashDividend, CashPerShare: 100}}
			v3, err := repo.Publish(ctx, b)
			require.NoError(t, err)
			ids, err = repo.DirtyInstruments(ctx, v2, v3)
			require.NoError(t, err)
			require.Equal(t, []market.InstrumentID{b.Instrument}, ids)

		})
	}
}
