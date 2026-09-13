package mysql

import (
	"context"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

func backtestRunRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"run_id", "kind", "status", "lease_owner", "lease_token", "request_json", "data_version", "strategy_id", "strategy_version", "engine_version"}).AddRow("Run A ", "BACKTEST", "RUNNING", "owner ", "token ", []byte(`{"instrument":{"Exchange":"SSE","Code":"600000"},"parameters":{"hold":2},"config":{"LotSize":100}}`), 1, "strategy", "v1", "v1")
}
func storedBacktestRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"run_id", "instrument_id", "exchange", "code", "total_return", "maximum_drawdown", "closed_trades", "has_open_position"}).AddRow("Run A ", 41, "SSE", "600000", 0.1, 0.2, 2, true)
}
func testBacktestResult(n int) backtest.Result {
	result := backtest.Result{Summary: backtest.Summary{MaximumDrawdown: 0.2, ClosedTrades: 2, HasOpenPosition: true}}
	at := testSnapshot().Key.AsOf
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("Order %d ", i)
		result.Orders = append(result.Orders, backtest.Order{ID: id, Side: backtest.Buy, Quantity: 100, CreatedAt: at, AttemptedAt: at.Add(time.Hour), Reason: "reason", FinalReason: backtest.OrderFilled})
		result.Fills = append(result.Fills, backtest.Fill{ID: "Fill " + id, OrderID: id, Side: backtest.Buy, Time: at.Add(time.Hour), Price: 123450, Quantity: 100, Gross: 12345000, Commission: 50, StampDuty: 12, TransferFee: 3})
		result.Equity = append(result.Equity, backtest.EquityPoint{Time: at.Add(time.Duration(i) * time.Second), Equity: 10000000, Cash: 5000000, PositionValue: 5000000})
	}
	return result
}
func TestBacktestWritesAllResultBatchesAndRollsBack(t *testing.T) {
	for _, stage := range []string{"summary", "orders", "fills", "equity", "outbox", "terminal", "success"} {
		t.Run(stage, func(t *testing.T) {
			repo, m := mockRepository(t)
			m.ExpectBegin()
			m.ExpectQuery("SELECT .*t_compute_runs").WillReturnRows(backtestRunRows())
			m.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
			failed := false
			for _, step := range []struct {
				name, table string
				count       int
			}{{"summary", "t_backtest_runs", 1}, {"orders", "t_backtest_orders", 2}, {"fills", "t_backtest_trades", 2}, {"equity", "t_backtest_equity_points", 2}} {
				if failed {
					break
				}
				for i := 0; i < step.count; i++ {
					e := m.ExpectExec("INSERT INTO `" + step.table + "`")
					if stage == step.name && (i == 1 || step.count == 1) {
						e.WillReturnError(errors.New("batch failed"))
						failed = true
						break
					}
					e.WillReturnResult(sqlmock.NewResult(1, 1))
				}
			}
			if !failed {
				m.ExpectQuery("SELECT UTC_TIMESTAMP").WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(testSnapshot().Key.AsOf))
				e := m.ExpectExec("INSERT INTO `t_outbox_events`")
				if stage == "outbox" {
					e.WillReturnError(errors.New("outbox failed"))
					failed = true
				} else {
					e.WillReturnResult(sqlmock.NewResult(1, 1))
				}
			}
			if !failed {
				e := m.ExpectExec("UPDATE `t_compute_runs`")
				if stage == "terminal" {
					e.WillReturnResult(sqlmock.NewResult(0, 0))
					failed = true
				} else {
					e.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if failed {
				m.ExpectRollback()
			} else {
				m.ExpectCommit()
			}
			err := NewRunStore(repo.db).CompleteBacktest(context.Background(), "Run A ", "token ", testBacktestResult(1001))
			if failed {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestBacktestInstrumentAndResultValidation(t *testing.T) {
	result := testBacktestResult(1)
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	result.Orders[0].Instrument = id
	input, err := decodeBacktestInputs([]byte(`{}`), result)
	require.NoError(t, err)
	require.Equal(t, id, input.Instrument)
	require.JSONEq(t, `{}`, string(input.Parameters))
	result.Orders[0].Instrument = market.InstrumentID{}
	result.Fills[0].Instrument = id
	input, err = decodeBacktestInputs([]byte(`{}`), result)
	require.NoError(t, err)
	require.Equal(t, id, input.Instrument)
	for _, raw := range []string{`{`, `{}`, `{"instrument":{"Exchange":"SZSE","Code":"000001"}}`} {
		_, err := decodeBacktestInputs([]byte(raw), backtest.Result{})
		if raw == `{"instrument":{"Exchange":"SZSE","Code":"000001"}}` {
			_, err = decodeBacktestInputs([]byte(raw), result)
		}
		require.Error(t, err)
	}
	result.Fills[0].Instrument = market.InstrumentID{Code: "bad"}
	_, err = decodeBacktestInputs([]byte(`{}`), result)
	require.Error(t, err)
	for _, change := range []func(*backtest.Result){func(r *backtest.Result) { r.Summary.MaximumDrawdown = math.NaN() }, func(r *backtest.Result) { r.Summary.ClosedTrades = -1 }, func(r *backtest.Result) { r.Orders[0].ID = "" }, func(r *backtest.Result) { r.Orders = append(r.Orders, r.Orders[0]) }, func(r *backtest.Result) { r.Fills[0].OrderID = "missing" }, func(r *backtest.Result) { r.Fills = append(r.Fills, r.Fills[0]) }, func(r *backtest.Result) { r.Equity[0].Time = time.Time{} }, func(r *backtest.Result) { r.Equity = append(r.Equity, r.Equity[0]) }} {
		r := testBacktestResult(1)
		change(&r)
		require.Error(t, validateBacktestResult(r))
	}
	require.NoError(t, validateBacktestResult(testBacktestResult(1)))
}
func TestBacktestPaginationPreservesOpaqueIDsMoneyAndCursor(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewRunStore(repo.db)
	ctx := context.Background()
	at := testSnapshot().Key.AsOf
	m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(storedBacktestRows())
	summary, err := s.BacktestResult(ctx, "Run A ")
	require.NoError(t, err)
	require.InDelta(t, 0.1, *summary.TotalReturn, 0.0001)
	require.Equal(t, 2, summary.ClosedTrades)
	m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(storedBacktestRows())
	m.ExpectQuery("SELECT .*t_backtest_orders.*sequence > .*ORDER BY sequence LIMIT").WithArgs("Run A ", int64(0), 2).WillReturnRows(sqlmock.NewRows([]string{"sequence", "order_id", "created_time", "attempted_at", "side", "final_reason", "quantity", "reason"}).AddRow(7, "Order ", at, at.Add(time.Hour), "1", "1", 100, "reason").AddRow(8, "Order2", at, nil, "1", "2", 100, "end"))
	orders, err := s.Orders(ctx, "Run A ", port.PageRequest{Limit: 1})
	require.NoError(t, err)
	require.EqualValues(t, 7, *orders.NextSequence)
	require.Equal(t, "Order ", orders.Items[0].ID)
	require.Equal(t, market.InstrumentID{Exchange: market.SSE, Code: "600000"}, orders.Items[0].Instrument)
	require.Equal(t, at.Add(time.Hour), orders.Items[0].AttemptedAt)
	m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(storedBacktestRows())
	m.ExpectQuery("SELECT .*t_backtest_trades").WillReturnRows(sqlmock.NewRows([]string{"sequence", "fill_id", "order_id", "side", "time", "price", "quantity", "gross", "commission", "stamp_duty", "transfer_fee"}).AddRow(5, "Fill ", "Order ", "2", at, 123450, 100, 12345000, 50, 12, 3).AddRow(6, "Fill2", "Order2", "2", at, 1, 1, 1, 0, 0, 0))
	fills, err := s.Trades(ctx, "Run A ", port.PageRequest{Limit: 1})
	require.NoError(t, err)
	require.EqualValues(t, 5, *fills.NextSequence)
	require.Equal(t, "Fill ", fills.Items[0].ID)
	require.Equal(t, market.Money(12345000), fills.Items[0].Gross)
	require.Equal(t, market.Money(12), fills.Items[0].StampDuty)
	m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(storedBacktestRows())
	m.ExpectQuery("SELECT .*t_backtest_equity_points").WillReturnRows(sqlmock.NewRows([]string{"sequence", "time", "cash", "position_value", "equity"}).AddRow(3, at, 5, 6, 11).AddRow(4, at.Add(time.Hour), 6, 7, 13))
	points, err := s.Equity(ctx, "Run A ", port.PageRequest{Limit: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, *points.NextSequence)
	require.Equal(t, market.Money(11), points.Items[0].Equity)
}
func TestBacktestReadsRejectMissingCorruptAndFailedStorage(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewRunStore(repo.db)
	ctx := context.Background()
	_, err := s.BacktestResult(ctx, "")
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	for index, rows := range []*sqlmock.Rows{emptyRows(), storedBacktestRows()} {
		m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(rows)
		if index > 0 {
			m.ExpectQuery("SELECT .*t_backtest_orders").WillReturnError(errors.New("read error"))
		}
		_, err := s.Orders(ctx, "run", port.PageRequest{Limit: 1})
		require.Error(t, err)
	}
	for _, column := range []string{"side", "final_reason"} {
		m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(storedBacktestRows())
		rows := sqlmock.NewRows([]string{"side", "final_reason"})
		if column == "side" {
			rows.AddRow("bad", "1")
		} else {
			rows.AddRow("1", "bad")
		}
		m.ExpectQuery("SELECT .*t_backtest_orders").WillReturnRows(rows)
		_, err := s.Orders(ctx, "run", port.PageRequest{Limit: 1})
		require.Error(t, err)
	}
	m.ExpectQuery("SELECT .*t_backtest_runs").WillReturnRows(storedBacktestRows())
	m.ExpectQuery("SELECT .*t_backtest_trades").WillReturnRows(sqlmock.NewRows([]string{"side"}).AddRow("bad"))
	_, err = s.Trades(ctx, "run", port.PageRequest{Limit: 1})
	require.Error(t, err)
}
