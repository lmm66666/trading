package mysql

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

func longLegacyFixture(n int) (legacyInstrument, migrationSource) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	item := legacyInstrument{ID: id, Bars: map[market.Timeframe][]model.StockKline{}}
	source := migrationSource{bars: map[market.Timeframe][]market.Bar{}}
	for i := 0; i < n; i++ {
		at := time.Date(2000, 1, 1+i, 0, 0, 0, 0, time.UTC)
		item.Bars[market.Day] = append(item.Bars[market.Day], model.StockKline{Code: id.Code, Date: at.Format("2006-01-02"), Open: 10, High: 11, Low: 9, Close: 10, Volume: 100})
		source.bars[market.Day] = append(source.bars[market.Day], market.Bar{Instrument: id, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: 100000, High: 110000, Low: 90000, Close: 100000, Volume: 100})
		source.factors = append(source.factors, market.AdjustmentFactor{EffectiveTime: at, Numerator: int64(i + 1), Denominator: 1})
	}
	return item, source
}

func targetStateRows(t *testing.T, state legacyTargetCursor) *sqlmock.Rows {
	t.Helper()
	payload, err := json.Marshal(state)
	require.NoError(t, err)
	return sqlmock.NewRows([]string{"payload"}).AddRow(payload)
}

type targetPosition struct {
	positions [4]int
	verified  bool
}

func (want targetPosition) Match(value driver.Value) bool {
	payload, ok := value.([]byte)
	if !ok {
		return false
	}
	var state legacyTargetCursor
	return json.Unmarshal(payload, &state) == nil && state.Positions == want.positions && state.Verified == want.verified
}
func expectTargetCheckpoint(mock sqlmock.Sqlmock, id market.InstrumentID, positions [4]int, verified bool) {
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WithArgs("target", id.String(), uint64(0), targetPosition{positions, verified}, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
}
func expectOneLegacyBar(mock sqlmock.Sqlmock, bar market.Bar) *sqlmock.ExpectedExec {
	return mock.ExpectExec("INSERT INTO `t_market_bars`").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(41), "DAY", bar.OpenTime, bar.CloseTime, uint32(1), uint64(1), nil, int64(bar.Open), int64(bar.High), int64(bar.Low), int64(bar.Close), bar.Volume, int64(bar.Amount), "TRADABLE", nil, nil)
}
func legacyStoredBars(bars []market.Bar, start int) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "instrument_id", "timeframe", "open_time", "close_time", "revision", "valid_from_version", "open", "high", "low", "close", "volume", "amount", "trading_status"})
	for i, b := range bars {
		rows.AddRow(start+i, 41, timeframeName(b.Timeframe), b.OpenTime, b.CloseTime, 1, 1, int64(b.Open), int64(b.High), int64(b.Low), int64(b.Close), b.Volume, int64(b.Amount), "TRADABLE")
	}
	return rows
}

func TestLegacyTargetFailureResumesAfterCommittedBoundedBatch(t *testing.T) {
	repo, mock := mockRepository(t)
	item, source := longLegacyFixture(3)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	state := legacyTargetCursor{InstrumentID: 41, Digest: batch.Digest}
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, state))
	mock.ExpectBegin()
	expectOneLegacyBar(mock, batch.Bars[market.Day][0]).WillReturnResult(sqlmock.NewResult(1, 1))
	expectTargetCheckpoint(mock, item.ID, [4]int{1}, false)
	mock.ExpectCommit()
	failure := errors.New("second batch failed")
	mock.ExpectBegin()
	expectOneLegacyBar(mock, batch.Bars[market.Day][1]).WillReturnError(failure)
	mock.ExpectRollback()
	require.ErrorIs(t, writeLegacyInstrument(repo.db, 1, item, batch, 1), failure)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WithArgs(market.DataVersion(1), versionComplete, 1).WillReturnRows(emptyRows())
	_, err = completeVersion(repo.db, 1)
	require.Error(t, err)
	// 恢复只允许第二根开始的 INSERT；重复写第一根会使参数断言失败。
	state.Positions[0] = 1
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, state))
	for index := 1; index < 3; index++ {
		mock.ExpectBegin()
		expectOneLegacyBar(mock, batch.Bars[market.Day][index]).WillReturnResult(sqlmock.NewResult(int64(index+1), 1))
		expectTargetCheckpoint(mock, item.ID, [4]int{index + 1}, false)
		mock.ExpectCommit()
	}
	for index := range batch.Factors {
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO `t_adjustment_factors`").WillReturnResult(sqlmock.NewResult(int64(index+1), 1))
		expectTargetCheckpoint(mock, item.ID, [4]int{3, 0, index + 1}, false)
		mock.ExpectCommit()
	}
	for i, b := range batch.Bars[market.Day] {
		mock.ExpectQuery("SELECT .*t_market_bars.*LIMIT").WithArgs(uint64(41), uint64(1), uint64(i), 1).WillReturnRows(legacyStoredBars([]market.Bar{b}, i+1))
	}
	mock.ExpectQuery("SELECT .*t_market_bars.*LIMIT").WillReturnRows(emptyRows())
	for i, f := range batch.Factors {
		mock.ExpectQuery("SELECT .*t_adjustment_factors.*LIMIT").WillReturnRows(sqlmock.NewRows([]string{"id", "effective_time", "numerator", "denominator"}).AddRow(i+1, f.EffectiveTime, f.Numerator, f.Denominator))
	}
	mock.ExpectQuery("SELECT .*t_adjustment_factors.*LIMIT").WillReturnRows(emptyRows())
	mock.ExpectQuery("SELECT .*t_corporate_actions.*LIMIT").WillReturnRows(emptyRows())
	mock.ExpectBegin()
	expectTargetCheckpoint(mock, item.ID, [4]int{3, 0, 3}, true)
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, writeLegacyInstrument(repo.db, 1, item, batch, 1))
}

func TestLegacyFailureCategoriesNeverIncludeProviderDetails(t *testing.T) {
	for _, tc := range []struct {
		err      error
		category string
	}{{errors.New("password@tcp(secret)/db"), "SOURCE_UNAVAILABLE"}, {ErrMigrationIncomplete, "INCOMPLETE_DATA"}, {ErrLegacyChanged, "INVALID_DATA"}, {context.Canceled, "CANCELED"}, {port.ErrInvalidPortValue, "INVALID_DATA"}} {
		require.Equal(t, tc.category, legacyFailureCategory(tc.err))
	}
}

func TestRegularPublishCannotCrossIncompleteMigrationVersion(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	expectVersionLock(mock)
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(emptyRows())
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(migrationVersionRows(1, "INCOMPLETE"))
	mock.ExpectRollback()
	_, err := repo.Publish(context.Background(), testBatch(100000))
	require.ErrorIs(t, err, ErrLegacyMigrationBusy)
}

func oneSourceInfo() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(4, "600000", "浦发")
}
func oneSourceDaily() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "code", "date", "open", "high", "low", "close", "volume"}).AddRow(9, "600000", "2024-01-02", 10, 11, 9, 10, 100)
}
func expectSourceScan(mock sqlmock.Sqlmock, dry bool) {
	mock.ExpectBegin()
	for index, table := range []string{"t_stock_info", "t_stock_kline_daily", "t_stock_kline_weekly"} {
		if !dry {
			mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
		}
		q := mock.ExpectQuery("SELECT .*"+table+".*ORDER BY id ASC.*LIMIT").WithArgs(uint64(0), 100)
		switch index {
		case 0:
			q.WillReturnRows(oneSourceInfo())
		case 1:
			q.WillReturnRows(oneSourceDaily())
		case 2:
			q.WillReturnRows(emptyRows())
		}
		if !dry {
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .*t_legacy_kernel_migration.*LIMIT").WillReturnRows(emptyRows())
			if index < 2 {
				expectPutState(mock)
			}
			expectPutState(mock)
			mock.ExpectCommit()
		}
	}
	mock.ExpectCommit()
}
func expectPutState(mock sqlmock.Sqlmock) {
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
}
func expectLoadOneInstrument(t *testing.T, mock sqlmock.Sqlmock, dry bool) {
	t.Helper()
	item, _ := legacyFixture(t)
	for index, stage := range legacyStages {
		if dry {
			q := mock.ExpectQuery("SELECT .*t_stock_")
			switch index {
			case 0:
				q.WillReturnRows(oneSourceInfo())
			case 1:
				q.WillReturnRows(oneSourceDaily())
			case 2:
				q.WillReturnRows(emptyRows())
			}
			continue
		}
		rows := sqlmock.NewRows([]string{"legacy_id", "payload"})
		if index < 2 {
			var value any = item.Bars[market.Day][0]
			if index == 0 {
				value = model.StockInfo{Code: item.ID.Code, Name: item.Name}
			}
			payload, err := json.Marshal(value)
			require.NoError(t, err)
			rows.AddRow(index+1, payload)
		}
		mock.ExpectQuery("SELECT .*t_legacy_kernel_migration.*ORDER BY legacy_id ASC.*LIMIT").WithArgs(stage, item.ID.String(), uint64(0), 100).WillReturnRows(rows)
	}
}
func expectInitialTarget(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .*t_market_data_versions.*ORDER BY version DESC").WillReturnRows(versionRows(0))
	for _, table := range []string{"t_market_bars", "t_adjustment_factors", "t_corporate_actions"} {
		mock.ExpectQuery("SELECT count.*" + table).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}
}
func expectMigrationStart(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	expectInitialTarget(mock)
	expectLegacySchema(mock)
	mock.ExpectBegin()
	expectVersionLock(mock)
	expectInitialTarget(mock)
	mock.ExpectExec("INSERT INTO `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	expectPutState(mock)
	mock.ExpectCommit()
}
func expectMigrationRelease(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT RELEASE_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
}

func TestLegacyStreamingRunCompletesAfterVerifiedTargetBatches(t *testing.T) {
	repo, mock := mockRepository(t)
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	expectMigrationStart(mock)
	expectSourceScan(mock, false)
	expectPutState(mock)
	expectLoadOneInstrument(t, mock, false)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	expectPutState(mock)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_instruments`").WillReturnResult(sqlmock.NewResult(41, 1))
	expectPutState(mock)
	mock.ExpectCommit()
	mock.ExpectBegin()
	expectOneLegacyBar(mock, batch.Bars[market.Day][0]).WillReturnResult(sqlmock.NewResult(1, 1))
	expectTargetCheckpoint(mock, item.ID, [4]int{1}, false)
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `t_adjustment_factors`").WillReturnResult(sqlmock.NewResult(1, 1))
	expectTargetCheckpoint(mock, item.ID, [4]int{1, 0, 1}, false)
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .*t_market_bars.*LIMIT").WillReturnRows(legacyStoredBars(batch.Bars[market.Day], 1))
	mock.ExpectQuery("SELECT .*t_adjustment_factors.*LIMIT").WillReturnRows(sqlmock.NewRows([]string{"id", "effective_time", "numerator", "denominator"}).AddRow(1, batch.Factors[0].EffectiveTime, 1, 1))
	mock.ExpectQuery("SELECT .*t_corporate_actions.*LIMIT").WillReturnRows(emptyRows())
	mock.ExpectBegin()
	expectTargetCheckpoint(mock, item.ID, [4]int{1, 0, 1}, true)
	expectPutState(mock)
	mock.ExpectCommit()
	expectPutState(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WillReturnRows(migrationVersionRows(1, "INCOMPLETE"))
	mock.ExpectQuery("SELECT count.*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(0, 1))
	expectPutState(mock)
	mock.ExpectCommit()
	expectMigrationRelease(mock)
	report, err := NewLegacyMigrator(repo.db, source).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.NoError(t, err)
	require.Equal(t, port.DataComplete, report.Quality)
	require.True(t, report.BacktestEnabled)
	require.Equal(t, int64(1), report.CompletedInstruments)
	require.Equal(t, int64(1), report.DailyBarCount)
	require.Len(t, report.SourceDigest, 64)
	require.Len(t, report.Digest, 64)
}

func TestLegacyDryRunFailureReportsInstrumentAndStableCategory(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			repo, mock := mockRepository(t)
			_, source := legacyFixture(t)
			if failed {
				source.err = errors.New("account:password@tcp(secret)/db")
			}
			expectSourceScan(mock, true)
			expectLoadOneInstrument(t, mock, true)
			report, err := NewLegacyMigrator(repo.db, source).Run(context.Background(), MigrationOptions{DryRun: true, BatchSize: 100})
			require.False(t, report.BacktestEnabled)
			require.Zero(t, report.Version)
			if failed {
				require.ErrorIs(t, err, ErrMigrationIncomplete)
				require.Equal(t, []MigrationFailure{{Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600000"}, Category: "SOURCE_UNAVAILABLE"}}, report.Failures)
				encoded, err := json.Marshal(report)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "password")
				require.Equal(t, port.DataIncomplete, report.Quality)
			} else {
				require.NoError(t, err)
				require.Equal(t, port.DataComplete, report.Quality)
			}
		})
	}
}

func TestLegacyRunRejectsExistingMarketVersionBeforeSchemaOrSourceRead(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	expectMigrationRelease(mock)
	report, err := NewLegacyMigrator(repo.db, nil).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.ErrorIs(t, err, ErrLegacyTargetNotEmpty)
	require.Equal(t, port.DataIncomplete, report.Quality)
	require.False(t, report.BacktestEnabled)
}
func TestLegacyRunReturnsSameCompletedReportWithoutSourceOrTargetWrites(t *testing.T) {
	repo, mock := mockRepository(t)
	want := MigrationReport{Version: 1, Quality: port.DataComplete, BacktestEnabled: true, DailyBarCount: 9, Digest: "digest"}
	payload, err := json.Marshal(want)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(migrationVersionRows(1, versionComplete))
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
	expectMigrationRelease(mock)
	got, err := NewLegacyMigrator(repo.db, nil).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLegacySourceReplayRejectsDriftWithoutMutatingCheckpoint(t *testing.T) {
	base := legacyMigrationRecord{Stage: "daily", InstrumentKey: "SSE:600000", LegacyID: 9, Payload: []byte("original")}
	for _, tc := range []struct {
		name       string
		page       []legacyMigrationRecord
		stored     []legacyMigrationRecord
		committed  uint64
		checkpoint bool
		wantErr    bool
	}{
		{"same prefix", []legacyMigrationRecord{base}, []legacyMigrationRecord{base}, 9, true, false},
		{"shorter replay", []legacyMigrationRecord{base}, []legacyMigrationRecord{base}, 10, false, false},
		{"deletion", nil, []legacyMigrationRecord{base}, 9, false, true},
		{"addition to frozen source", []legacyMigrationRecord{base}, nil, ^uint64(0), false, true},
		{"changed value", []legacyMigrationRecord{{Stage: base.Stage, InstrumentKey: base.InstrumentKey, LegacyID: 9, Payload: []byte("changed")}}, []legacyMigrationRecord{base}, 9, false, true},
		{"changed key", []legacyMigrationRecord{{Stage: base.Stage, InstrumentKey: "BSE:920001", LegacyID: 9, Payload: base.Payload}}, []legacyMigrationRecord{base}, 9, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := mockRepository(t)
			mock.ExpectBegin()
			rows := sqlmock.NewRows([]string{"stage", "instrument_key", "legacy_id", "payload"})
			for _, r := range tc.stored {
				rows.AddRow(r.Stage, r.InstrumentKey, r.LegacyID, r.Payload)
			}
			mock.ExpectQuery("SELECT .*t_legacy_kernel_migration.*LIMIT").WillReturnRows(rows)
			if tc.wantErr {
				mock.ExpectRollback()
			} else {
				if tc.checkpoint {
					expectPutState(mock)
				}
				mock.ExpectCommit()
			}
			err := persistLegacySourcePage(repo.db, "daily", 1, 0, tc.committed, tc.page, legacySourceCursor{LastID: 9}, 10)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrLegacyChanged)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLegacySourceCountsDailyWeeklyAndRejectsUnknownCodes(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_stock_info").WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(4, "600000").AddRow(8, "920001").AddRow(10, "123456"))
	mock.ExpectQuery("SELECT .*t_stock_kline_daily").WillReturnRows(oneSourceDaily())
	mock.ExpectQuery("SELECT .*t_stock_kline_weekly").WillReturnRows(sqlmock.NewRows([]string{"id", "code", "date", "open", "high", "low", "close", "volume"}).AddRow(12, "920001", "2024-01-05", 10, 11, 9, 10, 100))
	mock.ExpectCommit()
	ids, report, err := streamLegacySource(repo.db, repo.db, MigrationOptions{DryRun: true, BatchSize: 100}, 0, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), report.InstrumentCount)
	require.Equal(t, market.BSE, ids[0].Exchange)
	require.Equal(t, int64(1), report.WeeklyBarCount)
	require.Equal(t, "2024-01-05", report.WeeklyDates.To.Format("2006-01-02"))
	require.Equal(t, []string{"123456"}, report.RejectedCodes)
}

func TestLegacyPreparedFailurePersistsSafePerInstrumentDiagnostic(t *testing.T) {
	repo, mock := mockRepository(t)
	_, source := legacyFixture(t)
	source.actionErr = errors.New("private password in upstream URL")
	expectSourceScan(mock, false)
	expectPutState(mock)
	expectLoadOneInstrument(t, mock, false)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	expectPutState(mock)
	expectPutState(mock)
	report, err := NewLegacyMigrator(repo.db, source).migratePrepared(repo.db, MigrationOptions{BatchSize: 100}, MigrationReport{Version: 1})
	require.ErrorIs(t, err, ErrMigrationIncomplete)
	require.Equal(t, port.DataIncomplete, report.Quality)
	require.False(t, report.BacktestEnabled)
	require.Equal(t, "SOURCE_UNAVAILABLE", report.Failures[0].Category)
	require.Equal(t, "600000", report.Failures[0].Instrument.Code)
}

func TestLegacyCachedVerifiedTargetNeedsNoProviderOrMarketRewrites(t *testing.T) {
	repo, mock := mockRepository(t)
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	payload, err := json.Marshal(batch)
	require.NoError(t, err)
	source.err = errors.New("source offline")
	expectSourceScan(mock, false)
	expectPutState(mock)
	expectLoadOneInstrument(t, mock, false)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, legacyTargetCursor{InstrumentID: 41, Digest: batch.Digest, Verified: true}))
	expectPutState(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WillReturnRows(migrationVersionRows(1, "INCOMPLETE"))
	mock.ExpectQuery("SELECT count.*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(0, 1))
	expectPutState(mock)
	mock.ExpectCommit()
	report, err := NewLegacyMigrator(repo.db, source).migratePrepared(repo.db, MigrationOptions{BatchSize: 100}, MigrationReport{Version: 1})
	require.NoError(t, err)
	require.Equal(t, port.DataComplete, report.Quality)
}

func TestLegacyRunHandlesBusyLockAndReleaseFailureConservatively(t *testing.T) {
	for _, acquire := range []int{0, 1} {
		t.Run(fmt.Sprint(acquire), func(t *testing.T) {
			repo, mock := mockRepository(t)
			mock.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(acquire))
			if acquire == 1 {
				mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(migrationVersionRows(1, versionComplete))
				payload, _ := json.Marshal(MigrationReport{Version: 1, Quality: port.DataComplete, BacktestEnabled: true})
				mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
				mock.ExpectQuery("SELECT RELEASE_LOCK").WillReturnError(errors.New("connection lost"))
				mock.ExpectClose()
			} else {
				mock.ExpectQuery("SELECT RELEASE_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))
			}
			report, err := NewLegacyMigrator(repo.db, nil).Run(context.Background(), MigrationOptions{BatchSize: 100})
			require.ErrorIs(t, err, ErrLegacyMigrationBusy)
			require.Equal(t, port.DataIncomplete, report.Quality)
			require.False(t, report.BacktestEnabled)
		})
	}
}

func TestLegacyInitialTargetRejectsAnyOrphanMarketRows(t *testing.T) {
	for fail := 0; fail < 3; fail++ {
		repo, mock := mockRepository(t)
		mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(0))
		for i, table := range []string{"t_market_bars", "t_adjustment_factors", "t_corporate_actions"} {
			q := mock.ExpectQuery("SELECT count.*" + table)
			count := 0
			if i == fail {
				count = 1
			}
			q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			if i == fail {
				break
			}
		}
		_, err := inspectLegacyTarget(repo.db)
		require.ErrorIs(t, err, ErrLegacyTargetNotEmpty)
	}
}

func TestLegacyReadStateRejectsMalformedCheckpoint(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte("bad json")))
	var state legacyTargetCursor
	_, err := readMigrationState(repo.db, "target", "SSE:600000", 0, &state)
	require.Error(t, err)
}

func TestLegacyApplyRejectsSingleConnectionPoolBeforeTakingLock(t *testing.T) {
	repo, _ := mockRepository(t)
	pool, err := repo.db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	_, err = NewLegacyMigrator(repo.db, nil).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
}

func TestLegacyAmbiguousLockAcquisitionStillReleasesDedicatedConnection(t *testing.T) {
	repo, mock := mockRepository(t)
	sentinel := errors.New("response lost after acquisition")
	mock.ExpectQuery("SELECT GET_LOCK").WillReturnError(sentinel)
	expectMigrationRelease(mock)
	_, err := NewLegacyMigrator(repo.db, nil).Run(context.Background(), MigrationOptions{BatchSize: 100})
	require.ErrorIs(t, err, sentinel)
}

func TestLegacyPreparationRecoversReservedVersionAndRejectsEveryDBFailure(t *testing.T) {
	patterns := []string{"SELECT .*t_market_data_versions.*FOR UPDATE", "SELECT .*t_market_data_versions.*ORDER BY version DESC", "SELECT count.*t_market_bars", "SELECT count.*t_adjustment_factors", "SELECT count.*t_corporate_actions", "INSERT INTO `t_market_data_versions`", "SELECT .*t_legacy_kernel_migration", "INSERT INTO `t_legacy_kernel_migration`"}
	for fail := range patterns {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			repo, mock := mockRepository(t)
			mock.ExpectBegin()
			sentinel := errors.New("storage unavailable")
			for i, p := range patterns {
				if i > fail {
					break
				}
				if i == 5 || i == 7 {
					e := mock.ExpectExec(p)
					if i == fail {
						e.WillReturnError(sentinel)
					} else {
						e.WillReturnResult(sqlmock.NewResult(1, 1))
					}
				} else {
					q := mock.ExpectQuery(p)
					if i == fail {
						q.WillReturnError(sentinel)
					} else {
						rows := emptyRows()
						if i < 2 {
							rows = versionRows(0)
						} else if i < 5 {
							rows = sqlmock.NewRows([]string{"count"}).AddRow(0)
						}
						q.WillReturnRows(rows)
					}
				}
			}
			mock.ExpectRollback()
			report, err := prepareLegacyMigration(repo.db)
			require.ErrorIs(t, err, sentinel)
			require.False(t, report.BacktestEnabled)
		})
	}
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	expectVersionLock(mock)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(migrationVersionRows(1, "INCOMPLETE"))
	for _, table := range []string{"t_market_bars", "t_adjustment_factors", "t_corporate_actions"} {
		mock.ExpectQuery("SELECT count.*" + table).WithArgs(uint64(1)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}
	want := MigrationReport{Version: 1, Quality: port.DataIncomplete, SourceDigest: "snapshot"}
	payload, _ := json.Marshal(want)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
	mock.ExpectCommit()
	got, err := prepareLegacyMigration(repo.db)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLegacyFinalSwitchRejectsMissingProofOrFailedUpdate(t *testing.T) {
	for fail := 0; fail < 5; fail++ {
		repo, mock := mockRepository(t)
		report := MigrationReport{Version: 1, InstrumentCount: 1, Quality: port.DataIncomplete}
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WillReturnRows(migrationVersionRows(1, "INCOMPLETE"))
		q := mock.ExpectQuery("SELECT count.*t_legacy_kernel_migration")
		if fail == 0 {
			q.WillReturnError(errors.New("count failed"))
		} else if fail == 1 {
			q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		} else {
			q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			e := mock.ExpectExec("UPDATE `t_market_data_versions`")
			if fail == 2 {
				e.WillReturnError(errors.New("update failed"))
			} else if fail == 3 {
				e.WillReturnResult(sqlmock.NewResult(0, 0))
			} else {
				e.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnError(errors.New("report failed"))
			}
		}
		mock.ExpectRollback()
		require.Error(t, completeLegacyMigration(repo.db, &report))
		require.Equal(t, port.DataIncomplete, report.Quality)
		require.False(t, report.BacktestEnabled)
	}
}

func testLegacyBatchWithActions(t *testing.T) port.MarketWriteBatch {
	t.Helper()
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	weekly := batch.Bars[market.Day][0]
	weekly.Timeframe = market.Week
	batch.Bars[market.Week] = []market.Bar{weekly}
	batch.Actions = []market.CorporateAction{{ID: "cash", Instrument: item.ID, Kind: market.CashDividend, ExDate: weekly.CloseTime, CashPerShare: 100}}
	batch.Digest = ""
	batch, _, err = canonicalBatch(batch)
	require.NoError(t, err)
	return batch
}
func expectedLegacyVerificationRows(batch port.MarketWriteBatch) []*sqlmock.Rows {
	bars := append(append([]market.Bar{}, batch.Bars[market.Day]...), batch.Bars[market.Week]...)
	factors := sqlmock.NewRows([]string{"id", "effective_time", "numerator", "denominator"})
	for i, f := range batch.Factors {
		factors.AddRow(i+1, f.EffectiveTime, f.Numerator, f.Denominator)
	}
	actions := sqlmock.NewRows([]string{"id", "source_event_id", "ex_date", "kind", "cash_per_share"})
	for i, a := range batch.Actions {
		actions.AddRow(i+1, a.ID, a.ExDate, "CASH_DIVIDEND", int64(a.CashPerShare))
	}
	return []*sqlmock.Rows{legacyStoredBars(bars, 1), factors, actions}
}
func TestLegacyTargetVerifiesWeeklyAndActionsAndRejectsCorruption(t *testing.T) {
	batch := testLegacyBatchWithActions(t)
	repo, mock := mockRepository(t)
	mock.ExpectExec("INSERT INTO `t_market_bars`").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, insertLegacyTargetChunk(repo.db, 41, 1, batch, 1, 0, 1))
	mock.ExpectExec("INSERT INTO `t_corporate_actions`").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, insertLegacyTargetChunk(repo.db, 41, 1, batch, 3, 0, 1))
	require.Error(t, insertLegacyTargetChunk(repo.db, 41, 1, batch, 9, 0, 0))
	for fail := -1; fail < 4; fail++ {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			repo, mock := mockRepository(t)
			rows := expectedLegacyVerificationRows(batch)
			for i, table := range []string{"t_market_bars", "t_adjustment_factors", "t_corporate_actions"} {
				q := mock.ExpectQuery("SELECT .*"+table+".*LIMIT").WithArgs(uint64(41), uint64(1), uint64(0), 100)
				if i == fail {
					q.WillReturnError(errors.New("read unavailable"))
					break
				}
				q.WillReturnRows(rows[i])
			}
			expected := batch
			if fail == 3 {
				expected.Digest = "corrupted"
			}
			err := verifyLegacyInstrument(repo.db, 41, 1, expected, 100)
			if fail == -1 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestLegacyTargetRejectsInvalidOrUncommittedCursor(t *testing.T) {
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	for _, state := range []legacyTargetCursor{{InstrumentID: 41, Digest: "different"}, {InstrumentID: 41, Digest: batch.Digest, Positions: [4]int{-1}}, {InstrumentID: 41, Digest: batch.Digest, Positions: [4]int{2}}} {
		repo, mock := mockRepository(t)
		mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, state))
		require.ErrorIs(t, writeLegacyInstrument(repo.db, 1, item, batch, 1), ErrLegacyChanged)
	}
	repo, mock := mockRepository(t)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, legacyTargetCursor{InstrumentID: 41, Digest: batch.Digest}))
	mock.ExpectBegin()
	expectOneLegacyBar(mock, batch.Bars[market.Day][0]).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnError(errors.New("checkpoint write failed"))
	mock.ExpectRollback()
	require.Error(t, writeLegacyInstrument(repo.db, 1, item, batch, 1))
}

func TestLegacyFrozenSourceReplayKeepsDigestAndDoesNotWrite(t *testing.T) {
	repo, mock := mockRepository(t)
	expectSourceScan(mock, true)
	_, first, err := streamLegacySource(repo.db, repo.db, MigrationOptions{DryRun: true, BatchSize: 100}, 1, false)
	require.NoError(t, err)
	item, _ := legacyFixture(t)
	info := model.StockInfo{Code: item.ID.Code, Name: item.Name}
	info.ID = 4
	day := item.Bars[market.Day][0]
	day.ID = 9
	mock.ExpectBegin()
	for index, stage := range legacyStages {
		mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
		q := mock.ExpectQuery("SELECT .*t_stock_")
		switch index {
		case 0:
			q.WillReturnRows(oneSourceInfo())
		case 1:
			q.WillReturnRows(oneSourceDaily())
		case 2:
			q.WillReturnRows(emptyRows())
		}
		mock.ExpectBegin()
		rows := sqlmock.NewRows([]string{"stage", "instrument_key", "legacy_id", "payload"})
		if index < 2 {
			var value any = info
			id := 4
			if index == 1 {
				value = day
				id = 9
			}
			payload, err := json.Marshal(value)
			require.NoError(t, err)
			rows.AddRow(stage, item.ID.String(), id, payload)
		}
		mock.ExpectQuery("SELECT .*t_legacy_kernel_migration.*LIMIT").WillReturnRows(rows)
		mock.ExpectCommit()
	}
	mock.ExpectCommit()
	_, second, err := streamLegacySource(repo.db, repo.db, MigrationOptions{BatchSize: 100}, 1, true)
	require.NoError(t, err)
	require.Equal(t, first.SourceDigest, second.SourceDigest)
	require.Equal(t, first.DailyBarCount, second.DailyBarCount)
}

func TestLegacyPreparedStorageFailuresKeepInstrumentAndDisableBacktest(t *testing.T) {
	for _, stage := range []string{"load", "cache-read", "cache-write", "target"} {
		t.Run(stage, func(t *testing.T) {
			repo, mock := mockRepository(t)
			item, source := legacyFixture(t)
			sentinel := errors.New("private driver credentials")
			expectSourceScan(mock, false)
			expectPutState(mock)
			if stage == "load" {
				mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnError(sentinel)
			} else {
				expectLoadOneInstrument(t, mock, false)
				q := mock.ExpectQuery("SELECT .*t_legacy_kernel_migration")
				if stage == "cache-read" {
					q.WillReturnError(sentinel)
				} else {
					q.WillReturnRows(emptyRows())
					if stage == "cache-write" {
						mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnError(sentinel)
					} else {
						expectPutState(mock)
						mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnError(sentinel)
					}
				}
			}
			expectPutState(mock)
			report, err := NewLegacyMigrator(repo.db, source).migratePrepared(repo.db, MigrationOptions{BatchSize: 100}, MigrationReport{Version: 1})
			require.ErrorIs(t, err, sentinel)
			require.Equal(t, port.DataIncomplete, report.Quality)
			require.False(t, report.BacktestEnabled)
			require.Equal(t, []MigrationFailure{{Instrument: item.ID, Category: "STORAGE_FAILURE"}}, report.Failures)
		})
	}
}

func TestLegacyCachedForeignSourceIsRejectedWithoutTargetWrites(t *testing.T) {
	repo, mock := mockRepository(t)
	item, source := legacyFixture(t)
	batch, err := backfillLegacy(context.Background(), source, item)
	require.NoError(t, err)
	batch.Source = "foreign-source"
	batch.Digest = ""
	batch, _, err = canonicalBatch(batch)
	require.NoError(t, err)
	payload, err := json.Marshal(batch)
	require.NoError(t, err)
	expectSourceScan(mock, false)
	expectPutState(mock)
	expectLoadOneInstrument(t, mock, false)
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
	expectPutState(mock)
	expectPutState(mock)
	report, err := NewLegacyMigrator(repo.db, source).migratePrepared(repo.db, MigrationOptions{BatchSize: 100}, MigrationReport{Version: 1})
	require.ErrorIs(t, err, ErrMigrationIncomplete)
	require.Equal(t, "INVALID_DATA", report.Failures[0].Category)
	require.False(t, report.BacktestEnabled)
}

func TestLegacySourceCheckpointsFirstBoundedBatchBeforeNextRead(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(emptyRows())
	mock.ExpectQuery("SELECT .*t_stock_info.*ORDER BY id ASC.*LIMIT").WithArgs(uint64(0), 2).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(3, "600000", "甲").AddRow(9, "920001", "乙"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_legacy_kernel_migration.*legacy_id.*LIMIT").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	failure := errors.New("stop after the durable first batch")
	mock.ExpectQuery("SELECT .*t_stock_info.*ORDER BY id ASC.*LIMIT").WithArgs(uint64(9), 2).WillReturnError(failure)
	mock.ExpectRollback()
	_, report, err := streamLegacySource(repo.db, repo.db, MigrationOptions{BatchSize: 2}, 1, false)
	require.ErrorIs(t, err, failure)
	require.Equal(t, uint64(9), report.LastLegacyIDs["info"])
	require.Equal(t, int64(2), report.InstrumentCount)
}

func TestLegacyFailedFinalCommitCannotReturnCompleteReport(t *testing.T) {
	repo, mock := mockRepository(t)
	report := MigrationReport{Version: 1, Quality: "COMPLETE", BacktestEnabled: true, InstrumentCount: 1, Digest: "digest"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WillReturnRows(migrationVersionRows(1, "INCOMPLETE"))
	mock.ExpectQuery("SELECT count.*t_legacy_kernel_migration").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `t_legacy_kernel_migration`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	err := completeLegacyMigration(repo.db, &report)
	require.Error(t, err)
	require.Equal(t, port.DataIncomplete, report.Quality)
	require.False(t, report.BacktestEnabled)
}
func TestLegacyLongHistoryAllocationsScaleLinearly(t *testing.T) {
	measure := func(n int) int64 {
		item, source := longLegacyFixture(n)
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := backfillLegacy(context.Background(), source, item)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		return result.AllocedBytesPerOp()
	}
	small, large := measure(100), measure(1000)
	require.Less(t, large, small*20, "10倍历史长度不能产生二次方级因子复制；small=%d large=%d", small, large)
}
