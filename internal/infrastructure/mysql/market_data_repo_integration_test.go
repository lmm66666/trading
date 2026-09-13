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
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
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
	for _, image := range []string{"mysql:5.7", "mysql:8.0"} {
		t.Run(image, func(t *testing.T) {
			db := dbtest.StartMySQL(t, image)
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
			var selects atomic.Int64
			repo = NewMarketDataRepository(db.Session(&gorm.Session{Logger: selectCounter{Interface: db.Logger, count: &selects}}))
			req := requestFor(b, v)
			req.Auxiliary = []market.Timeframe{market.Week, market.Month}
			req.LookbackBars = 1
			bundles, errs := repo.BatchDatasets(ctx, ids, req)
			require.Len(t, errs, 0)
			require.Len(t, bundles, 5000)
			require.LessOrEqual(t, selects.Load(), int64(8))
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
	db := dbtest.StartMySQL(t, "mysql:8.0")
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
}
