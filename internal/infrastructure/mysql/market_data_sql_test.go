package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

// SQL mocks exercise our query/mapping/error boundaries when Docker is absent.
// Real isolation, DDL and locking are verified only by integration-tag tests.
func mockRepository(t *testing.T) (*MarketDataRepository, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(driver.New(driver.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() }})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = conn.Close() })
	return NewMarketDataRepository(db), mock
}

func versionRows(v uint64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "version", "instrument_id", "source", "status", "quality", "digest"}).AddRow(v+1, v, 41, "fixture", versionComplete, "COMPLETE", "")
}
func instrumentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "exchange", "code", "name", "board", "active", "lot_size", "source"}).AddRow(41, "SSE", "600000", "", "", false, 0, "fixture")
}
func barRows(b market.Bar) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "instrument_id", "timeframe", "open_time", "close_time", "revision", "valid_from_version", "open", "high", "low", "close", "volume", "amount", "trading_status", "limit_up"}).AddRow(7, 41, timeframeName(b.Timeframe), b.OpenTime, b.CloseTime, 1, 1, int64(b.Open), int64(b.High), int64(b.Low), int64(b.Close), b.Volume, int64(b.Amount), "TRADABLE", 110000)
}
func emptyRows() *sqlmock.Rows { return sqlmock.NewRows([]string{"id"}) }

func TestBatchSQLMapsIndependentDatasetsAndWarmup(t *testing.T) {
	repo, mock := mockRepository(t)
	b := testBatch(100000)
	req := port.BatchRequest{PrimaryTimeframe: market.Day, Auxiliary: []market.Timeframe{market.Week}, From: b.Bars[market.Day][0].OpenTime, To: b.Bars[market.Day][0].CloseTime, Version: 2, LookbackBars: 1}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(2))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(3))
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
	mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(barRows(b.Bars[market.Day][0]))
	warmup := b.Bars[market.Day][0]
	warmup.OpenTime = warmup.OpenTime.AddDate(0, 0, -1)
	warmup.CloseTime = warmup.CloseTime.AddDate(0, 0, -1)
	mock.ExpectQuery("\\(SELECT .*t_market_bars").WillReturnRows(barRows(warmup))
	mock.ExpectQuery("SELECT .*t_adjustment_factors").WillReturnRows(sqlmock.NewRows([]string{"instrument_id", "effective_time", "numerator", "denominator"}).AddRow(41, warmup.OpenTime, 1, 1))
	mock.ExpectQuery("SELECT .*t_corporate_actions").WillReturnRows(sqlmock.NewRows([]string{"instrument_id", "source_event_id", "ex_date", "kind", "cash_per_share"}).AddRow(41, "dividend", warmup.OpenTime, "CASH_DIVIDEND", 100))
	mock.ExpectCommit()
	missing := market.InstrumentID{Exchange: market.SSE, Code: "600001"}
	got, failures := repo.BatchDatasets(context.Background(), []market.InstrumentID{b.Instrument, missing, b.Instrument}, req)
	require.Len(t, got, 1)
	require.ErrorIs(t, failures[missing], gorm.ErrRecordNotFound)
	bundle := got[b.Instrument]
	require.Equal(t, 2, bundle.Primary.Len())
	require.Equal(t, market.DataVersion(2), bundle.Primary.Bar(0).Version)
	require.Len(t, bundle.Auxiliary, 1)
	require.Len(t, bundle.Factors, 1)
	require.Len(t, bundle.Actions, 1)
	bar := bundle.Primary.Bar(0)
	*bar.LimitUp = 1
	require.Equal(t, market.Price(110000), *bundle.Primary.Bar(0).LimitUp)
}

func TestBatchSQLPropagatesEveryReadFailure(t *testing.T) {
	for failedStep := 0; failedStep < 6; failedStep++ {
		t.Run(string(rune('a'+failedStep)), func(t *testing.T) {
			repo, mock := mockRepository(t)
			b := testBatch(100000)
			req := port.BatchRequest{PrimaryTimeframe: market.Day, From: b.Bars[market.Day][0].OpenTime, To: b.Bars[market.Day][0].CloseTime, Version: 1}
			patterns := []string{"SELECT .*t_market_data_versions", "SELECT .*t_market_data_versions", "SELECT .*t_instruments", "SELECT .*t_market_bars", "SELECT .*t_adjustment_factors", "SELECT .*t_corporate_actions"}
			rows := []*sqlmock.Rows{versionRows(1), versionRows(1), instrumentRows(), barRows(b.Bars[market.Day][0]), emptyRows(), emptyRows()}
			mock.ExpectBegin()
			for i := 0; i <= failedStep; i++ {
				q := mock.ExpectQuery(patterns[i])
				if i == failedStep {
					q.WillReturnError(errors.New("read unavailable"))
				} else {
					q.WillReturnRows(rows[i])
				}
			}
			mock.ExpectRollback()
			got, errs := repo.BatchDatasets(context.Background(), []market.InstrumentID{b.Instrument}, req)
			require.Empty(t, got)
			require.ErrorContains(t, errs[b.Instrument], "read unavailable")
		})
	}
}

func TestBatchInvalidInputAndUnknownInstruments(t *testing.T) {
	repo, mock := mockRepository(t)
	ctx := context.Background()
	b := testBatch(100000)
	req := port.BatchRequest{PrimaryTimeframe: market.Day, From: b.Bars[market.Day][0].OpenTime, To: b.Bars[market.Day][0].CloseTime, Version: 1}
	got, errs := repo.BatchDatasets(ctx, nil, req)
	require.Empty(t, got)
	require.Empty(t, errs)
	_, errs = repo.BatchDatasets(ctx, make([]market.InstrumentID, 5001), req)
	require.NotEmpty(t, errs)
	_, errs = repo.BatchDatasets(ctx, []market.InstrumentID{b.Instrument}, port.BatchRequest{})
	require.Error(t, errs[b.Instrument])
	_, errs = repo.BatchDatasets(ctx, []market.InstrumentID{{}}, req)
	require.Error(t, errs[market.InstrumentID{}])
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(emptyRows())
	mock.ExpectCommit()
	_, _, _, err := repo.Dataset(ctx, b.Instrument, market.Day, req.From, req.To, 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestMarketVersionAndInstrumentSQL(t *testing.T) {
	repo, mock := mockRepository(t)
	ctx := context.Background()
	mock.ExpectQuery("SELECT .*t_market_data_versions.*version > 0 AND status").WillReturnRows(versionRows(7))
	v, err := repo.LatestCompleteVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, market.DataVersion(7), v)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnError(gorm.ErrRecordNotFound)
	_, err = repo.LatestCompleteVersion(ctx)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = completeVersion(repo.db, 0)
	require.Error(t, err)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(sqlmock.NewRows([]string{"version", "quality"}).AddRow(1, "BAD"))
	_, err = completeVersion(repo.db, 1)
	require.Error(t, err)
	_, err = repo.Instruments(ctx, port.InstrumentScope{})
	require.Error(t, err)
	mock.ExpectQuery("SELECT .*t_instruments.*exchange IN.*active").WillReturnRows(instrumentRows())
	ids, err := repo.Instruments(ctx, port.InstrumentScope{Exchanges: []market.Exchange{market.SSE}, ActiveOnly: true, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, []market.InstrumentID{{Exchange: market.SSE, Code: "600000"}}, ids)
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnError(errors.New("unavailable"))
	_, err = repo.Instruments(ctx, port.InstrumentScope{Limit: 10})
	require.Error(t, err)
	_, err = repo.DirtyInstruments(ctx, 2, 1)
	require.Error(t, err)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(2))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT i.*UNION SELECT instrument_id.*UNION SELECT instrument_id").WithArgs(uint64(1), uint64(2), uint64(1), uint64(2), uint64(1), uint64(2)).WillReturnRows(instrumentRows())
	ids, err = repo.DirtyInstruments(ctx, 1, 2)
	require.NoError(t, err)
	require.Len(t, ids, 1)
}

func TestPublishSQLClosesAllChangedKindsBeforeCompletion(t *testing.T) {
	repo, mock := mockRepository(t)
	b := testBatch(110000)
	at := b.Bars[market.Day][0].OpenTime
	b.Factors = []market.AdjustmentFactor{{EffectiveTime: at, Numerator: 2, Denominator: 1}}
	b.Actions = []market.CorporateAction{{ID: "dividend", Instrument: b.Instrument, ExDate: at, Kind: market.CashDividend, CashPerShare: 200}}
	mock.ExpectBegin()
	expectVersionLock(mock)
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectExec("INSERT INTO `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(barRows(testBatch(100000).Bars[market.Day][0]))
	mock.ExpectExec("UPDATE `t_market_bars`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `t_market_bars`").WillReturnResult(sqlmock.NewResult(8, 1))
	mock.ExpectQuery("SELECT .*t_adjustment_factors").WillReturnRows(sqlmock.NewRows([]string{"id", "instrument_id", "effective_time", "numerator", "denominator"}).AddRow(9, 41, at, 1, 1))
	mock.ExpectExec("UPDATE `t_adjustment_factors`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `t_adjustment_factors`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery("SELECT .*t_corporate_actions").WillReturnRows(sqlmock.NewRows([]string{"id", "instrument_id", "source_event_id", "ex_date", "kind", "cash_per_share"}).AddRow(11, 41, "dividend", at, "CASH_DIVIDEND", 100))
	mock.ExpectExec("UPDATE `t_corporate_actions`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `t_corporate_actions`").WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec("UPDATE `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	v, err := repo.Publish(context.Background(), b)
	require.NoError(t, err)
	require.Equal(t, market.DataVersion(2), v)
}

func expectVersionLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .*t_market_data_versions.*FOR UPDATE").WithArgs(versionLock, versionLockSource, 1).WillReturnRows(sqlmock.NewRows([]string{"version", "source", "status"}).AddRow(0, versionLockSource, versionLock))
}

func TestPublishSQLDigestIdempotencyAndUnknownMetadata(t *testing.T) {
	repo, mock := mockRepository(t)
	b := testBatch(100000)
	digest, err := MarketBatchDigest(b)
	require.NoError(t, err)
	mock.ExpectBegin()
	expectVersionLock(mock)
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(sqlmock.NewRows([]string{"version", "digest"}).AddRow(4, digest))
	mock.ExpectCommit()
	v, err := repo.Publish(context.Background(), b)
	require.NoError(t, err)
	require.Equal(t, market.DataVersion(4), v)
	mock.ExpectBegin()
	expectVersionLock(mock)
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_instruments`").WithArgs(
		sqlmock.AnyArg(), sqlmock.AnyArg(), "SSE", "600000", "EQUITY", "SPOT_EQUITY", "", nil, nil,
		int64(0), int64(0), "", "", false, int64(0), "fixture",
	).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(emptyRows())
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(0))
	mock.ExpectExec("INSERT INTO `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(emptyRows())
	mock.ExpectExec("INSERT INTO `t_market_bars`").WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec("UPDATE `t_market_data_versions`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	v, err = repo.Publish(context.Background(), b)
	require.NoError(t, err)
	require.Equal(t, market.DataVersion(1), v)
}

func TestPublishSQLRollsBackAtEachMutationFailure(t *testing.T) {
	for _, stage := range []string{"pending", "close", "insert", "complete", "commit"} {
		t.Run(stage, func(t *testing.T) {
			repo, mock := mockRepository(t)
			b := testBatch(110000)
			injected := errors.New("injected database failure")
			mock.ExpectBegin()
			expectVersionLock(mock)
			mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
			mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
			mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
			pending := mock.ExpectExec("INSERT INTO `t_market_data_versions`")
			if stage == "pending" {
				pending.WillReturnError(injected)
			} else {
				pending.WillReturnResult(sqlmock.NewResult(3, 1))
				mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(barRows(testBatch(100000).Bars[market.Day][0]))
				close := mock.ExpectExec("UPDATE `t_market_bars`")
				if stage == "close" {
					close.WillReturnError(injected)
				} else {
					close.WillReturnResult(sqlmock.NewResult(0, 1))
					insert := mock.ExpectExec("INSERT INTO `t_market_bars`")
					if stage == "insert" {
						insert.WillReturnError(injected)
					} else {
						insert.WillReturnResult(sqlmock.NewResult(8, 1))
						complete := mock.ExpectExec("UPDATE `t_market_data_versions`")
						if stage == "complete" {
							complete.WillReturnError(injected)
						} else {
							complete.WillReturnResult(sqlmock.NewResult(0, 1))
						}
					}
				}
			}
			if stage == "commit" {
				mock.ExpectCommit().WillReturnError(injected)
			} else {
				mock.ExpectRollback()
			}
			v, err := repo.Publish(context.Background(), b)
			require.Zero(t, v)
			require.ErrorIs(t, err, injected)
		})
	}
}

func TestBatchCorruptInstrumentFailsIndependently(t *testing.T) {
	repo, mock := mockRepository(t)
	b := testBatch(100000)
	other := market.InstrumentID{Exchange: market.SSE, Code: "600001"}
	req := port.BatchRequest{PrimaryTimeframe: market.Day, From: b.Bars[market.Day][0].OpenTime, To: b.Bars[market.Day][0].CloseTime, Version: 1}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows().AddRow(42, "SSE", "600001", "", "", false, 0, "fixture"))
	bar := b.Bars[market.Day][0]
	mock.ExpectQuery("SELECT .*t_market_bars").WillReturnRows(barRows(bar).AddRow(8, 42, "DAY", bar.OpenTime, bar.CloseTime, 1, 1, 100000, 120000, 90000, 0, 100, 10000000, "TRADABLE", 110000))
	mock.ExpectQuery("SELECT .*t_adjustment_factors").WillReturnRows(emptyRows())
	mock.ExpectQuery("SELECT .*t_corporate_actions").WillReturnRows(emptyRows())
	mock.ExpectCommit()
	bundles, errs := repo.BatchDatasets(context.Background(), []market.InstrumentID{b.Instrument, other}, req)
	require.Len(t, bundles, 1)
	require.Equal(t, 1, bundles[b.Instrument].Primary.Len())
	require.ErrorIs(t, errs[other], market.ErrInvalidOHLC)
}
