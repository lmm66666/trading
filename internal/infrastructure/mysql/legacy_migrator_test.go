package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
	"math"
	"sync"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

func TestLegacyCodeMapping(t *testing.T) {
	for _, tc := range []struct {
		code     string
		exchange market.Exchange
	}{
		{"600000", market.SSE}, {"601000", market.SSE}, {"603000", market.SSE}, {"605000", market.SSE}, {"688000", market.SSE}, {"689000", market.SSE},
		{"000001", market.SZSE}, {"001001", market.SZSE}, {"002001", market.SZSE}, {"003001", market.SZSE}, {"300001", market.SZSE}, {"301001", market.SZSE},
		{"400001", market.BSE}, {"800001", market.BSE}, {"920001", market.BSE},
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

type migrationSource struct {
	bars      map[market.Timeframe][]market.Bar
	factors   []market.AdjustmentFactor
	err       error
	actions   []market.CorporateAction
	actionErr error
}

func (s migrationSource) FetchBars(_ context.Context, _ market.InstrumentID, tf market.Timeframe, _, _ time.Time) ([]market.Bar, []market.AdjustmentFactor, error) {
	return s.bars[tf], s.factors, s.err
}

func (s migrationSource) FetchCorporateActions(context.Context, market.InstrumentID) ([]market.CorporateAction, error) {
	return s.actions, s.actionErr
}

func legacyFixture(t *testing.T) (legacyInstrument, migrationSource) {
	t.Helper()
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	old := model.StockKline{Code: id.Code, Date: "2024-01-02", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100}
	b := market.Bar{Instrument: id, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: 100000, High: 110000, Low: 90000, Close: 100000, Volume: 100, Amount: 10000000}
	return legacyInstrument{ID: id, Name: "浦发", Bars: map[market.Timeframe][]model.StockKline{market.Day: {old}}}, migrationSource{bars: map[market.Timeframe][]market.Bar{market.Day: {b}}, factors: []market.AdjustmentFactor{{EffectiveTime: at, Numerator: 1, Denominator: 1}}}
}

func TestLegacyBackfillRequiresExactDatesAndAdjustmentCoverage(t *testing.T) {
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	require.Len(t, batch.Bars[market.Day], 1)
	require.Len(t, batch.Factors, 1)
	for _, tc := range []struct {
		name   string
		mutate func(*migrationSource)
	}{
		{"source error", func(s *migrationSource) { s.err = errors.New("offline") }},
		{"actions error", func(s *migrationSource) { s.actionErr = errors.New("offline") }},
		{"missing bars", func(s *migrationSource) { s.bars = nil }},
		{"missing factors", func(s *migrationSource) { s.factors = nil }},
		{"future factor", func(s *migrationSource) { s.factors[0].EffectiveTime = s.factors[0].EffectiveTime.AddDate(0, 0, 1) }},
		{"different date", func(s *migrationSource) {
			s.bars[market.Day][0].CloseTime = s.bars[market.Day][0].CloseTime.AddDate(0, 0, 1)
		}},
		{"bad OHLC", func(s *migrationSource) { s.bars[market.Day][0].Low = 120000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item, s := legacyFixture(t)
			tc.mutate(&s)
			_, err := backfillLegacy(context.Background(), s, item)
			require.Error(t, err)
		})
	}
	_, err = backfillLegacy(context.Background(), nil, item)
	require.Error(t, err)
}

func TestLegacyBarConversionRejectsCorruptValues(t *testing.T) {
	item, _ := legacyFixture(t)
	old := item.Bars[market.Day][0]
	got, err := legacyBar(old, market.Day)
	require.NoError(t, err)
	require.Equal(t, market.Price(100000), got.Close)
	for _, tc := range []struct {
		name   string
		mutate func(*model.StockKline)
	}{{"date", func(b *model.StockKline) { b.Date = "yesterday" }}, {"ohlc", func(b *model.StockKline) { b.Low = 20 }}, {"volume", func(b *model.StockKline) { b.Volume = -1 }}, {"code", func(b *model.StockKline) { b.Code = "123456" }}} {
		t.Run(tc.name, func(t *testing.T) { b := old; tc.mutate(&b); _, err := legacyBar(b, market.Day); require.Error(t, err) })
	}
}

func TestMigrationOptionsRejectInvalidBatchBeforeDBAccess(t *testing.T) {
	for _, n := range []int{0, -1, 10001} {
		_, err := NewLegacyMigrator(nil, nil).Run(context.Background(), MigrationOptions{BatchSize: n})
		require.Error(t, err)
	}
}

var _ port.MarketSource = migrationSource{}

func TestLegacyBackfillRejectsCrossTimeframeFactorConflict(t *testing.T) {
	item, source := legacyFixture(t)
	weekly := item.Bars[market.Day][0]
	weekly.Date = "2024-01-03"
	item.Bars[market.Week] = []model.StockKline{weekly}
	b := source.bars[market.Day][0]
	b.Timeframe = market.Week
	b.OpenTime = b.OpenTime.AddDate(0, 0, 1)
	b.CloseTime = b.OpenTime
	s := timeframeMigrationSource{day: source, week: migrationSource{bars: map[market.Timeframe][]market.Bar{market.Week: {b}}, factors: []market.AdjustmentFactor{{EffectiveTime: b.OpenTime.AddDate(0, 0, -2), Numerator: 1, Denominator: 2}}}}
	_, err := backfillLegacy(context.Background(), s, item)
	require.ErrorIs(t, err, ErrMigrationIncomplete)
}

type timeframeMigrationSource struct{ day, week migrationSource }

func (s timeframeMigrationSource) FetchBars(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error) {
	if tf == market.Week {
		return s.week.FetchBars(ctx, id, tf, from, to)
	}
	return s.day.FetchBars(ctx, id, tf, from, to)
}

func (s timeframeMigrationSource) FetchCorporateActions(ctx context.Context, id market.InstrumentID) ([]market.CorporateAction, error) {
	return s.day.FetchCorporateActions(ctx, id)
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

func TestLegacyBarRejectsNonFiniteOverflowAndHandlesSuspension(t *testing.T) {
	item, _ := legacyFixture(t)
	old := item.Bars[market.Day][0]
	for _, price := range []float64{math.NaN(), math.Inf(1), math.MaxFloat64, 0, -1} {
		b := old
		b.Open = price
		_, err := legacyBar(b, market.Day)
		require.Error(t, err)
	}
	old.Volume = 0
	got, err := legacyBar(old, market.Day)
	require.NoError(t, err)
	require.Equal(t, market.Suspended, got.Trading)
}

func TestLegacyBackfillRejectsDuplicateDatesAndEmptyHistory(t *testing.T) {
	item, source := legacyFixture(t)
	item.Bars[market.Day] = append(item.Bars[market.Day], item.Bars[market.Day][0])
	_, err := backfillLegacy(context.Background(), source, item)
	require.ErrorIs(t, err, market.ErrDuplicateBar)
	item.Bars = nil
	_, err = backfillLegacy(context.Background(), source, item)
	require.ErrorIs(t, err, ErrMigrationIncomplete)
	item, source = legacyFixture(t)
	item.Bars[market.Day][0].Date = "bad"
	_, err = backfillLegacy(context.Background(), source, item)
	require.Error(t, err)
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
