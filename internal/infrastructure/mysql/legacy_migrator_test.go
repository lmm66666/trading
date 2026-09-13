package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
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

func expectLegacyRead(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_stock_info.*id >.*ORDER BY id ASC.*LIMIT").WithArgs(uint64(0), 100).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(4, "600000", "浦发").AddRow(8, "123456", "未知"))
	mock.ExpectQuery("SELECT .*t_stock_kline_daily.*id >.*ORDER BY id ASC.*LIMIT").WithArgs(uint64(0), 100).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "date", "open", "high", "low", "close", "volume"}).AddRow(9, "600000", "2024-01-02", 10, 11, 9, 10, 100))
	mock.ExpectQuery("SELECT .*t_stock_kline_weekly.*id >.*ORDER BY id ASC.*LIMIT").WithArgs(uint64(0), 100).WillReturnRows(emptyRows())
	mock.ExpectCommit()
}
func TestLegacyDryRunDoesNotWriteOrCreateSchema(t *testing.T) {
	repo, mock := mockRepository(t)
	_, source := legacyFixture(t)
	expectLegacyRead(mock)
	report, err := NewLegacyMigrator(repo.db, source).Run(context.Background(), MigrationOptions{DryRun: true, BatchSize: 100})
	require.ErrorIs(t, err, ErrMigrationIncomplete)
	require.Equal(t, int64(1), report.InstrumentCount)
	require.Equal(t, int64(1), report.DailyBarCount)
	require.Equal(t, uint64(9), report.LastLegacyIDs["daily"])
	require.Equal(t, []string{"123456"}, report.RejectedCodes)
	require.Len(t, report.Digest, 64)
	require.False(t, report.BacktestEnabled)
	require.Zero(t, report.Version)
}
func TestLegacyCheckpointCannotOverwriteChangedSource(t *testing.T) {
	repo, mock := mockRepository(t)
	records := []legacyMigrationRecord{{Stage: "daily", LegacyID: 9, Payload: []byte(`{"close":10}`)}}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration.*stage").WillReturnRows(sqlmock.NewRows([]string{"stage", "legacy_id", "payload"}).AddRow("daily", 9, []byte(`{"close":11}`)))
	mock.ExpectRollback()
	err := stageLegacy(repo.db, records, 100)
	require.ErrorIs(t, err, ErrLegacyChanged)
}
func TestLegacyCompletedRestartReturnsIdenticalReport(t *testing.T) {
	repo, mock := mockRepository(t)
	want := MigrationReport{Version: 7, InstrumentCount: 1, DailyBarCount: 10, Quality: port.DataComplete, Digest: "abc", BacktestEnabled: true}
	payload, err := json.Marshal(want)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"stage", "legacy_id", "payload"}).AddRow("report", 0, payload))
	got, found, err := completedLegacyReport(repo.db)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, got)
}

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
func expectMigrationAllocator(mock sqlmock.Sqlmock, existing bool) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WillReturnRows(versionRows(0))
	previous := emptyRows()
	if existing {
		previous = migrationVersionRows(1, "INCOMPLETE")
	}
	mock.ExpectQuery("SELECT .*t_market_data_versions.*source").WillReturnRows(previous)
	latest := versionRows(0)
	if existing {
		latest = migrationVersionRows(1, "INCOMPLETE")
	}
	mock.ExpectQuery("SELECT .*t_market_data_versions.*ORDER BY version DESC").WillReturnRows(latest)
	if !existing {
		mock.ExpectExec("INSERT INTO `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(2, 1))
	}
}
func expectMigrationReportSave(mock sqlmock.Sqlmock, quality port.DataQuality, digest string) {
	update := mock.ExpectExec("UPDATE `t_market_data_versions`")
	if quality == port.DataComplete {
		update.WithArgs(digest, sqlmock.AnyArg(), string(quality), string(quality), sqlmock.AnyArg(), uint64(1))
	} else {
		update.WithArgs(digest, string(quality), string(quality), sqlmock.AnyArg(), uint64(1))
	}
	update.WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
func TestLegacyIncompleteVersionIsDurableAndUnreadable(t *testing.T) {
	repo, mock := mockRepository(t)
	report := MigrationReport{Quality: port.DataIncomplete, Digest: "incomplete"}
	expectMigrationAllocator(mock, false)
	expectMigrationReportSave(mock, port.DataIncomplete, "incomplete")
	require.NoError(t, finishLegacy(repo.db, &report, nil, nil))
	require.Equal(t, market.DataVersion(1), report.Version)
	require.False(t, report.BacktestEnabled)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WithArgs(market.DataVersion(1), versionComplete, 1).WillReturnRows(emptyRows())
	_, err := completeVersion(repo.db, 1)
	require.Error(t, err)
}
func TestLegacyCompletePublicationUsesOneVersionForAllRows(t *testing.T) {
	repo, mock := mockRepository(t)
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	report := MigrationReport{Quality: port.DataComplete, Digest: batch.Digest, BacktestEnabled: true}
	expectMigrationAllocator(mock, true)
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_instruments`").WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_market_bars`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT .*t_adjustment_factors").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_adjustment_factors`").WillReturnResult(sqlmock.NewResult(1, 1))
	expectMigrationReportSave(mock, port.DataComplete, batch.Digest)
	require.NoError(t, finishLegacy(repo.db, &report, []port.MarketWriteBatch{batch}, []legacyInstrument{item}))
	require.Equal(t, market.DataVersion(1), report.Version)
}
func TestLegacyCheckpointReplaySkipsCommittedRowsAndBatchesNewRows(t *testing.T) {
	repo, mock := mockRepository(t)
	records := []legacyMigrationRecord{{Stage: "daily", LegacyID: 7, Payload: []byte("old")}, {Stage: "daily", LegacyID: 10, Payload: []byte("new")}}
	for i, row := range records {
		mock.ExpectBegin()
		q := mock.ExpectQuery("SELECT .*t_legacy_kernel_migration")
		if i == 0 {
			q.WillReturnRows(sqlmock.NewRows([]string{"stage", "legacy_id", "payload"}).AddRow(row.Stage, row.LegacyID, row.Payload))
		} else {
			q.WillReturnRows(emptyRows())
			mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WithArgs(row.Stage, row.LegacyID, row.Payload, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectCommit()
	}
	require.NoError(t, stageLegacy(repo.db, records, 1))
}
func TestLegacyRunRestartDoesNotCallProviderOrReadLegacy(t *testing.T) {
	repo, mock := mockRepository(t)
	expectLegacySchema(mock)
	want := MigrationReport{Version: 7, InstrumentCount: 1, DailyBarCount: 10, Quality: port.DataComplete, Digest: "abc", BacktestEnabled: true}
	payload, err := json.Marshal(want)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"stage", "legacy_id", "payload"}).AddRow("report", 0, payload))
	got, err := NewLegacyMigrator(repo.db, nil).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.NoError(t, err)
	require.Equal(t, want, got)
}
func TestLegacyReadPropagatesEveryTableFailure(t *testing.T) {
	for fail := 0; fail < 3; fail++ {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			repo, mock := mockRepository(t)
			mock.ExpectBegin()
			for i, table := range []string{"t_stock_info", "t_stock_kline_daily", "t_stock_kline_weekly"} {
				q := mock.ExpectQuery("SELECT .*" + table)
				if i == fail {
					q.WillReturnError(errors.New("read failed"))
					break
				}
				q.WillReturnRows(emptyRows())
			}
			mock.ExpectRollback()
			_, _, _, err := readLegacy(repo.db, 10)
			require.Error(t, err)
		})
	}
}

func expectCleanLegacyRead(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_stock_info").WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(4, "600000", "浦发"))
	mock.ExpectQuery("SELECT .*t_stock_kline_daily").WillReturnRows(sqlmock.NewRows([]string{"id", "code", "date", "open", "high", "low", "close", "volume"}).AddRow(9, "600000", "2024-01-02", 10, 11, 9, 10, 100))
	mock.ExpectQuery("SELECT .*t_stock_kline_weekly").WillReturnRows(emptyRows())
	mock.ExpectCommit()
}
func expectNewStage(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
func expectAnyReport(mock sqlmock.Sqlmock) {
	mock.ExpectExec("UPDATE `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
func expectMigrationDataWrites(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
	mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_market_bars`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT .*t_adjustment_factors").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_adjustment_factors`").WillReturnResult(sqlmock.NewResult(1, 1))
}
func TestLegacyRunPersistsBackfillAndCompletesOrResumes(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			repo, mock := mockRepository(t)
			item, source := legacyFixture(t)
			batch, err := backfillLegacy(context.Background(), source, item)
			require.NoError(t, err)
			expectLegacySchema(mock)
			mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
			expectCleanLegacyRead(mock)
			expectNewStage(mock)
			expectNewStage(mock)
			expectMigrationAllocator(mock, cached)
			expectAnyReport(mock)
			q := mock.ExpectQuery("SELECT .*t_legacy_kernel_migration")
			if cached {
				payload, err := json.Marshal(batch)
				require.NoError(t, err)
				q.WillReturnRows(sqlmock.NewRows([]string{"stage", "legacy_id", "payload"}).AddRow("backfill", 1, payload))
				source.err = errors.New("provider must not be required after restart")
			} else {
				q.WillReturnRows(emptyRows())
				expectNewStage(mock)
			}
			expectMigrationAllocator(mock, true)
			expectMigrationDataWrites(mock)
			expectAnyReport(mock)
			report, err := NewLegacyMigrator(repo.db, source).Run(context.Background(), MigrationOptions{BatchSize: 100})
			require.NoError(t, err)
			require.Equal(t, port.DataComplete, report.Quality)
			require.True(t, report.BacktestEnabled)
			require.Equal(t, market.DataVersion(1), report.Version)
			require.Equal(t, int64(1), report.DailyBarCount)
			require.Len(t, report.Digest, 64)
		})
	}
}
func TestLegacyRunFailedBackfillPersistsIncompleteWithoutMarketRows(t *testing.T) {
	repo, mock := mockRepository(t)
	_, source := legacyFixture(t)
	source.actionErr = errors.New("upstream unavailable")
	expectLegacySchema(mock)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	expectCleanLegacyRead(mock)
	expectNewStage(mock)
	expectNewStage(mock)
	expectMigrationAllocator(mock, false)
	expectAnyReport(mock)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	expectMigrationAllocator(mock, true)
	expectAnyReport(mock)
	report, err := NewLegacyMigrator(repo.db, source).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.ErrorIs(t, err, ErrMigrationIncomplete)
	require.Equal(t, port.DataIncomplete, report.Quality)
	require.False(t, report.BacktestEnabled)
	require.Equal(t, market.DataVersion(1), report.Version)
}

func TestLegacyPublicationRollsBackOnEveryDatabaseFailure(t *testing.T) {
	patterns := []string{"SELECT .*t_market_data_versions.*FOR UPDATE", "SELECT .*t_market_data_versions.*source", "SELECT .*t_market_data_versions.*ORDER BY version DESC", "INSERT INTO `t_market_data_versions`", "SELECT .*t_instruments", "INSERT INTO `t_instruments`", "SELECT .*t_market_bars", "INSERT INTO `t_market_bars`", "SELECT .*t_adjustment_factors", "INSERT INTO `t_adjustment_factors`", "UPDATE `t_market_data_versions`", "INSERT INTO `t_legacy_kernel_migration`"}
	for fail := range patterns {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			repo, mock := mockRepository(t)
			item, source := legacyFixture(t)
			batch, err := backfillLegacy(context.Background(), source, item)
			require.NoError(t, err)
			report := MigrationReport{Quality: port.DataComplete, Digest: batch.Digest}
			mock.ExpectBegin()
			sentinel := errors.New("database unavailable")
			for i, p := range patterns {
				if strings.HasPrefix(p, "SELECT") {
					q := mock.ExpectQuery(p)
					if i == fail {
						q.WillReturnError(sentinel)
						break
					}
					rows := emptyRows()
					if i == 0 || i == 2 {
						rows = versionRows(0)
					}
					q.WillReturnRows(rows)
				} else {
					e := mock.ExpectExec(p)
					if i == fail {
						e.WillReturnError(sentinel)
						break
					}
					e.WillReturnResult(sqlmock.NewResult(41, 1))
				}
			}
			mock.ExpectRollback()
			require.ErrorIs(t, finishLegacy(repo.db, &report, []port.MarketWriteBatch{batch}, []legacyInstrument{item}), sentinel)
		})
	}
}

func TestLegacyReadUsesPrimaryKeyPaginationAndPreservesDateRanges(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	for _, step := range []struct {
		cursor uint64
		id     uint64
		code   string
	}{{0, 4, "600000"}, {4, 10, "920001"}, {10, 0, ""}} {
		q := mock.ExpectQuery("SELECT .*t_stock_info.*ORDER BY id ASC").WithArgs(step.cursor, 1)
		rows := sqlmock.NewRows([]string{"id", "code"})
		if step.id > 0 {
			rows.AddRow(step.id, step.code)
		}
		q.WillReturnRows(rows)
	}
	for _, step := range []struct {
		table      string
		cursor, id uint64
		code, date string
	}{{"daily", 0, 9, "600000", "2024-01-03"}, {"daily", 9, 20, "600000", "2024-01-02"}, {"daily", 20, 25, "123456", "2024-01-01"}, {"daily", 25, 0, "", ""}, {"weekly", 0, 5, "920001", "2024-01-05"}, {"weekly", 5, 0, "", ""}} {
		q := mock.ExpectQuery("SELECT .*t_stock_kline_"+step.table+".*ORDER BY id ASC").WithArgs(step.cursor, 1)
		rows := sqlmock.NewRows([]string{"id", "code", "date", "open", "high", "low", "close", "volume"})
		if step.id > 0 {
			rows.AddRow(step.id, step.code, step.date, 10, 11, 9, 10, 100)
		}
		q.WillReturnRows(rows)
	}
	mock.ExpectCommit()
	records, items, report, err := readLegacy(repo.db, 1)
	require.NoError(t, err)
	require.Len(t, records, 6)
	require.Len(t, items, 2)
	require.Equal(t, market.BSE, items[0].ID.Exchange)
	require.Equal(t, int64(2), report.DailyBarCount)
	require.Equal(t, int64(1), report.WeeklyBarCount)
	require.Equal(t, "2024-01-02", report.DailyDates.From.Format("2006-01-02"))
	require.Equal(t, "2024-01-03", report.DailyDates.To.Format("2006-01-02"))
	require.Equal(t, uint64(25), report.LastLegacyIDs["daily"])
	require.Equal(t, []string{"123456"}, report.RejectedCodes)
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
func TestLegacyReportRejectsUnreadableOrMalformedCheckpoint(t *testing.T) {
	for _, failure := range []bool{false, true} {
		repo, mock := mockRepository(t)
		q := mock.ExpectQuery("SELECT .*t_legacy_kernel_migration")
		if failure {
			q.WillReturnError(errors.New("database unavailable"))
		} else {
			q.WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte("invalid JSON")))
		}
		_, found, err := completedLegacyReport(repo.db)
		require.Error(t, err)
		require.False(t, found)
	}
}
func TestLegacyAllocatorRejectsCompleteOrExhaustedVersion(t *testing.T) {
	for _, completed := range []bool{false, true} {
		repo, mock := mockRepository(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WillReturnRows(versionRows(0))
		rows := emptyRows()
		if completed {
			rows = migrationVersionRows(1, versionComplete)
		}
		mock.ExpectQuery("SELECT .*t_market_data_versions.*source").WillReturnRows(rows)
		if !completed {
			mock.ExpectQuery("SELECT .*t_market_data_versions.*ORDER BY version DESC").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("18446744073709551615"))
		}
		mock.ExpectRollback()
		err := finishLegacy(repo.db, &MigrationReport{Quality: port.DataIncomplete}, nil, nil)
		require.Error(t, err)
	}
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
