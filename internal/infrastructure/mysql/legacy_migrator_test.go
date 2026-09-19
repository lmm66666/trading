package mysql

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"trading/internal/market"
	"trading/internal/port"
	"trading/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func TestLegacyCodeMapping(t *testing.T) {
	for _, tc := range []struct {
		code     string
		exchange market.Exchange
	}{
		{"600000", market.SSE}, {"601000", market.SSE}, {"603000", market.SSE}, {"605000", market.SSE}, {"688000", market.SSE}, {"689000", market.SSE},
		{"000001", market.SZSE}, {"001001", market.SZSE}, {"002001", market.SZSE}, {"003001", market.SZSE}, {"300001", market.SZSE}, {"301001", market.SZSE},
		{"400001", market.BSE}, {"800001", market.BSE}, {"302001", market.BSE}, {"920001", market.BSE},
	} {
		id, err := MapLegacyInstrument(tc.code)
		require.NoError(t, err)
		require.Equal(t, market.InstrumentID{Exchange: tc.exchange, Code: tc.code}, id)
	}
	for _, code := range []string{"123456", "900001", "200001", "60000", "6000000", "sh600000", "60000a", " 600000", "600000 ", ""} {
		_, err := MapLegacyInstrument(code)
		require.ErrorIs(t, err, ErrUnknownExchange)
	}
}

func legacyFixture(t *testing.T) legacyInstrument {
	t.Helper()
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	old := model.StockKline{Code: id.Code, Date: "2024-01-02", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100}
	return legacyInstrument{ID: id, Name: "浦发", Bars: map[market.Timeframe][]model.StockKline{market.Day: {old}}}
}

func TestLegacyBackfillConvertsRowsDirectly(t *testing.T) {
	item := legacyFixture(t)
	weekly := item.Bars[market.Day][0]
	weekly.Date = "2024-01-08"
	item.Bars[market.Week] = []model.StockKline{weekly}

	batch, err := backfillLegacy(item)

	require.NoError(t, err)
	require.Equal(t, legacyMigrationSource, batch.Source)
	require.Equal(t, item.ID, batch.Instrument)
	require.Empty(t, batch.Factors)
	require.Empty(t, batch.Actions)
	day := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	week := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	require.Equal(t, []market.Bar{{
		Instrument: item.ID, Timeframe: market.Day, OpenTime: day, CloseTime: day,
		Open: 100000, High: 110000, Low: 90000, Close: 100000, Volume: 100, Trading: market.Tradable,
	}}, batch.Bars[market.Day])
	require.Equal(t, []market.Bar{{
		Instrument: item.ID, Timeframe: market.Week, OpenTime: week, CloseTime: week,
		Open: 100000, High: 110000, Low: 90000, Close: 100000, Volume: 100, Trading: market.Tradable,
	}}, batch.Bars[market.Week])
	require.NotEmpty(t, batch.Digest)
}

func TestLegacyBackfillRejectsInvalidLegacyRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*legacyInstrument)
		want   error
	}{
		{"duplicate date", func(item *legacyInstrument) {
			item.Bars[market.Day] = append(item.Bars[market.Day], item.Bars[market.Day][0])
		}, market.ErrDuplicateBar},
		{"inverted OHLC", func(item *legacyInstrument) { item.Bars[market.Day][0].Low = 12 }, market.ErrInvalidOHLC},
		{"corrupt zero price", func(item *legacyInstrument) { item.Bars[market.Day][0].Open = 0 }, market.ErrInvalidOHLC},
		{"negative volume", func(item *legacyInstrument) { item.Bars[market.Day][0].Volume = -1 }, market.ErrNegativeVolume},
		{"bad date", func(item *legacyInstrument) { item.Bars[market.Day][0].Date = "bad" }, nil},
		{"unknown code", func(item *legacyInstrument) { item.Bars[market.Day][0].Code = "123456" }, ErrUnknownExchange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := legacyFixture(t)
			tc.mutate(&item)
			_, err := backfillLegacy(item)
			if tc.want == nil {
				require.Error(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}
	item := legacyFixture(t)
	item.Bars = nil
	_, err := backfillLegacy(item)
	require.ErrorIs(t, err, ErrMigrationIncomplete)
}

func TestLegacyPriceScalesDecimalToFixedPoint(t *testing.T) {
	value, err := legacyPrice(12.3456)
	require.NoError(t, err)
	require.Equal(t, market.Price(123456), value)
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e300, -1e300} {
		_, err := legacyPrice(invalid)
		require.ErrorIs(t, err, port.ErrInvalidPortValue)
	}
}

func TestMigrationOptionsRejectInvalidBatchBeforeDBAccess(t *testing.T) {
	for _, n := range []int{0, -1, 10001} {
		_, err := NewLegacyMigrator(nil).Run(context.Background(), MigrationOptions{BatchSize: n})
		require.Error(t, err)
	}
}

func expectLegacySchema(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("test"))
	mock.ExpectQuery("SELECT SCHEMA_NAME").WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).AddRow("test"))
	mock.ExpectQuery("SELECT count.*information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("CREATE TABLE `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 0))
}

func migrationVersionRows(version uint64, status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"version", "source", "status", "quality"}).AddRow(version, legacyMigrationSource, status, status)
}

func TestLegacyCheckpointSchemaHasUTCMicrosecondAuditFields(t *testing.T) {
	s, err := schema.Parse(&legacyMigrationRecord{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, name := range []string{"CreatedAt", "UpdatedAt"} {
		field := s.LookUpField(name)
		require.NotNil(t, field)
		require.Equal(t, "datetime(6)", field.TagSettings["TYPE"])
		require.True(t, field.NotNull)
	}
}
