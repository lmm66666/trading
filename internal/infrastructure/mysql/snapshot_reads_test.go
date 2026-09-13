package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

func snapshotMetadata(failures string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"snapshot_id", "run_id", "strategy_id", "strategy_version", "parameters_hash", "data_version", "as_of", "status", "failures_json"}).AddRow("snapshot", "Run A ", "strategy", "v1", "hash", 1, testSnapshot().Key.AsOf, "PARTIAL_SUCCEEDED", []byte(failures))
}
func TestLatestSnapshotReadsPublishedPartialResultsAndIndependentValues(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewSignalSnapshotStore(repo.db)
	key := testSnapshot().Key
	key.AsOf = time.Time{}
	key.SnapshotID = "snapshot"
	for i := 0; i < 2; i++ {
		m.ExpectQuery("SELECT .*t_signal_snapshots.*status IN .*snapshot_id = .*ORDER BY as_of DESC, data_version DESC, id DESC").WithArgs("strategy", "v1", "hash", "SUCCEEDED", "PARTIAL_SUCCEEDED", "snapshot", 1).WillReturnRows(snapshotMetadata(`[{"instrument":{"Exchange":"SSE","Code":"600001"},"failure":{"code":"DATA","message":"missing"}}]`))
		m.ExpectQuery("SELECT r.*, i.exchange, i.code.*JOIN t_instruments.*r.sequence >").WithArgs("snapshot", int64(2), 10).WillReturnRows(sqlmock.NewRows([]string{"sequence", "exchange", "code", "signal_time", "reason", "values_json"}).AddRow(3, "SSE", "600000", testSnapshot().Key.AsOf, "hit", []byte(`{"score":2}`)))
		result, err := s.Latest(context.Background(), key, port.PageRequest{AfterSequence: 2, Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Failures, 1)
		require.Equal(t, 2.0, result.Rows[0].Values["score"])
		result.Rows[0].Values["score"] = 99
	}
}
func TestLatestSnapshotFailureBoundaries(t *testing.T) {
	for _, stage := range []string{"missing", "read", "failures", "rows", "values", "validation"} {
		t.Run(stage, func(t *testing.T) {
			repo, m := mockRepository(t)
			query := m.ExpectQuery("SELECT .*t_signal_snapshots.*as_of =")
			switch stage {
			case "missing":
				query.WillReturnRows(emptyRows())
			case "read":
				query.WillReturnError(errors.New("read"))
			case "failures":
				query.WillReturnRows(snapshotMetadata(`invalid`))
			default:
				query.WillReturnRows(snapshotMetadata(`[]`))
				rows := m.ExpectQuery("SELECT r.*, i.exchange, i.code")
				if stage == "rows" {
					rows.WillReturnError(errors.New("rows"))
				} else {
					values := `{}`
					code := "600000"
					if stage == "values" {
						values = `invalid`
					}
					if stage == "validation" {
						code = "bad"
					}
					rows.WillReturnRows(sqlmock.NewRows([]string{"exchange", "code", "signal_time", "reason", "values_json"}).AddRow("SSE", code, testSnapshot().Key.AsOf, "hit", []byte(values)))
				}
			}
			result, err := NewSignalSnapshotStore(repo.db).Latest(context.Background(), testSnapshot().Key, port.PageRequest{Limit: 10})
			require.Error(t, err)
			require.Empty(t, result.ID)
			if stage == "missing" {
				require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			}
		})
	}
	repo, _ := mockRepository(t)
	_, err := NewSignalSnapshotStore(repo.db).Latest(context.Background(), testSnapshot().Key, port.PageRequest{})
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
}
func TestCompleteScanPartialStatusZeroAsOfAndUnknownInstruments(t *testing.T) {
	for _, stage := range []string{"partial", "unknown", "instrument_read", "clock", "mismatch"} {
		t.Run(stage, func(t *testing.T) {
			repo, m := mockRepository(t)
			snap := testSnapshot()
			snap.Key.AsOf = time.Time{}
			snap.Failures = map[market.InstrumentID]port.Failure{{Exchange: market.SSE, Code: "600001"}: {Code: "DATA", Message: "missing"}, {Exchange: market.SSE, Code: "600002"}: {Code: "DATA", Message: "missing"}}
			m.ExpectBegin()
			rows := runRows(port.RunRunning)
			if stage == "mismatch" {
				rows = backtestRunRows()
			}
			m.ExpectQuery("SELECT .*t_compute_runs").WillReturnRows(rows)
			if stage != "mismatch" {
				clock := m.ExpectQuery("SELECT UTC_TIMESTAMP")
				if stage == "clock" {
					clock.WillReturnError(errors.New("clock"))
				} else {
					clock.WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(testSnapshot().Key.AsOf))
					q := m.ExpectQuery("SELECT .*t_instruments")
					if stage == "instrument_read" {
						q.WillReturnError(errors.New("read"))
					} else {
						if stage == "unknown" {
							q.WillReturnRows(emptyRows())
						} else {
							q.WillReturnRows(instrumentRows())
						}
						m.ExpectExec("INSERT INTO `t_signal_snapshots`").WillReturnResult(sqlmock.NewResult(1, 1))
						if stage == "partial" {
							m.ExpectExec("INSERT INTO `t_signal_snapshot_rows`").WillReturnResult(sqlmock.NewResult(1, 1))
							m.ExpectExec("INSERT INTO `t_outbox_events`").WillReturnResult(sqlmock.NewResult(1, 1))
							m.ExpectExec("UPDATE `t_compute_runs`").WithArgs("", "", nil, "PARTIAL_SUCCEEDED", "Run A ", "owner ", "token ").WillReturnResult(sqlmock.NewResult(0, 1))
						}
					}
				}
			}
			if stage == "partial" {
				m.ExpectCommit()
			} else {
				m.ExpectRollback()
			}
			err := NewRunStore(repo.db).CompleteScan(context.Background(), snap.RunID, "token ", snap)
			if stage == "partial" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
func TestRunGetAndFail(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewRunStore(repo.db)
	ctx := context.Background()
	_, err := s.Get(ctx, "")
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	for _, stage := range []string{"missing", "read", "success"} {
		q := m.ExpectQuery("SELECT .*t_compute_runs")
		switch stage {
		case "missing":
			q.WillReturnRows(emptyRows())
		case "read":
			q.WillReturnError(errors.New("read"))
		default:
			q.WillReturnRows(sqlmock.NewRows([]string{"run_id", "status", "cancel_requested_at", "attempts"}).AddRow("Run A ", "CANCELLED", testSnapshot().Key.AsOf, 4))
		}
		run, err := s.Get(ctx, "Run A ")
		if stage == "success" {
			require.NoError(t, err)
			require.Equal(t, 4, run.Attempts)
			require.Equal(t, testSnapshot().Key.AsOf, *run.CancelRequestedAt)
		} else {
			require.Error(t, err)
		}
	}
	require.ErrorIs(t, s.Fail(ctx, "", "", port.Failure{}), port.ErrInvalidPortValue)
	require.ErrorIs(t, s.Fail(ctx, "run", "token", port.Failure{}), port.ErrInvalidPortValue)
	for _, stage := range []string{"lease", "clock", "outbox", "terminal", "success"} {
		m.ExpectBegin()
		q := m.ExpectQuery("SELECT .*t_compute_runs")
		if stage == "lease" {
			q.WillReturnRows(emptyRows())
		} else {
			q.WillReturnRows(runRows(port.RunRunning))
			clock := m.ExpectQuery("SELECT UTC_TIMESTAMP")
			if stage == "clock" {
				clock.WillReturnError(errors.New("clock"))
			} else {
				clock.WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(testSnapshot().Key.AsOf))
				e := m.ExpectExec("INSERT INTO `t_outbox_events`")
				if stage == "outbox" {
					e.WillReturnError(errors.New("outbox"))
				} else {
					e.WillReturnResult(sqlmock.NewResult(1, 1))
					e = m.ExpectExec("UPDATE `t_compute_runs`")
					if stage == "terminal" {
						e.WillReturnResult(sqlmock.NewResult(0, 0))
					} else {
						e.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
		}
		if stage == "success" {
			m.ExpectCommit()
		} else {
			m.ExpectRollback()
		}
		err := s.Fail(ctx, "Run A ", "token ", port.Failure{Code: "DATA", Message: "missing"})
		if stage == "success" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
func TestOutboxFailureAndConflict(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewOutbox(repo.db)
	event := port.Event{ID: "event", AggregateID: "run", Kind: "test", Payload: []byte("ok"), OccurredAt: testSnapshot().Key.AsOf}
	require.ErrorIs(t, s.Publish(context.Background(), port.Event{}), port.ErrInvalidPortValue)
	long := event
	long.Kind = strings.Repeat("x", 65)
	require.ErrorIs(t, s.Publish(context.Background(), long), port.ErrInvalidPortValue)
	for _, stage := range []string{"insert", "read", "conflict", "commit"} {
		m.ExpectBegin()
		e := m.ExpectExec("INSERT INTO `t_outbox_events`")
		if stage == "insert" {
			e.WillReturnError(errors.New("insert"))
		} else {
			e.WillReturnResult(sqlmock.NewResult(1, 1))
			q := m.ExpectQuery("SELECT .*t_outbox_events")
			if stage == "read" {
				q.WillReturnError(errors.New("read"))
			} else {
				payload := `"b2s="`
				if stage == "conflict" {
					payload = `"bm90b2s="`
				}
				q.WillReturnRows(sqlmock.NewRows([]string{"kind", "aggregate_id", "payload", "occurred_at"}).AddRow("test", "run", []byte(payload), event.OccurredAt))
			}
		}
		if stage == "commit" {
			m.ExpectCommit().WillReturnError(errors.New("commit"))
		} else {
			m.ExpectRollback()
		}
		require.Error(t, s.Publish(context.Background(), event))
	}
}
func TestCompletionRejectsInvalidInputsAndLease(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewRunStore(repo.db)
	snap := testSnapshot()
	snap.ID = ""
	require.Error(t, s.CompleteScan(context.Background(), "run", "token", snap))
	snap = testSnapshot()
	require.Error(t, s.CompleteScan(context.Background(), "other", "token", snap))
	for _, stage := range []string{"lease", "kind", "clock", "instrument", "request", "result"} {
		m.ExpectBegin()
		q := m.ExpectQuery("SELECT .*t_compute_runs")
		switch stage {
		case "lease":
			q.WillReturnRows(emptyRows())
		case "kind":
			q.WillReturnRows(runRows(port.RunRunning))
		case "request":
			q.WillReturnRows(sqlmock.NewRows([]string{"kind", "request_json"}).AddRow("BACKTEST", []byte(`{}`)))
		default:
			q.WillReturnRows(backtestRunRows())
			if stage != "result" {
				iq := m.ExpectQuery("SELECT .*t_instruments")
				if stage == "instrument" {
					iq.WillReturnError(errors.New("instrument"))
				} else {
					iq.WillReturnRows(instrumentRows())
					m.ExpectExec("INSERT INTO `t_backtest_runs`").WillReturnResult(sqlmock.NewResult(1, 1))
					m.ExpectQuery("SELECT UTC_TIMESTAMP").WillReturnError(errors.New("clock"))
				}
			}
		}
		m.ExpectRollback()
		result := backtest.Result{}
		if stage == "result" {
			result.Summary.ClosedTrades = -1
		}
		require.Error(t, s.CompleteBacktest(context.Background(), "run", "token", result))
	}
}
