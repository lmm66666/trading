package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

func testSnapshot() port.SignalSnapshot {
	return port.SignalSnapshot{ID: "snapshot", RunID: "Run A ", Key: port.SnapshotKey{StrategyID: "strategy", StrategyVersion: "v1", ParametersHash: "hash", AsOf: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, DataVersion: 1, Rows: []port.SnapshotRow{{Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600000"}, SignalTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Reason: "hit", Values: map[string]float64{"score": 2}}}}
}
func TestCompleteScanPublishesAtomicallyAndFencesStaleWorker(t *testing.T) {
	for _, stage := range []string{"lease", "snapshot", "rows", "outbox", "terminal", "commit", "success"} {
		t.Run(stage, func(t *testing.T) {
			repo, m := mockRepository(t)
			s := NewRunStore(repo.db)
			snap := testSnapshot()
			injected := errors.New("write failed")
			m.ExpectBegin()
			q := m.ExpectQuery("SELECT .*t_compute_runs.*FOR UPDATE")
			if stage == "lease" {
				q.WillReturnRows(emptyRows())
				m.ExpectRollback()
				require.ErrorIs(t, s.CompleteScan(context.Background(), snap.RunID, "token ", snap), port.ErrLeaseLost)
				return
			}
			q.WillReturnRows(runRows(port.RunRunning))
			m.ExpectQuery("SELECT UTC_TIMESTAMP\\(6\\)").WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(snap.Key.AsOf))
			m.ExpectQuery("SELECT .*t_instruments").WillReturnRows(instrumentRows())
			steps := []struct{ name, sql string }{{"snapshot", "INSERT INTO `t_signal_snapshots`"}, {"rows", "INSERT INTO `t_signal_snapshot_rows`"}, {"outbox", "INSERT INTO `t_outbox_events`"}, {"terminal", "UPDATE `t_compute_runs`"}}
			for _, step := range steps {
				e := m.ExpectExec(step.sql)
				if stage == step.name {
					e.WillReturnError(injected)
					break
				}
				e.WillReturnResult(sqlmock.NewResult(1, 1))
			}
			if stage == "success" {
				m.ExpectCommit()
			} else if stage == "commit" {
				m.ExpectCommit().WillReturnError(injected)
			} else {
				m.ExpectRollback()
			}
			err := s.CompleteScan(context.Background(), snap.RunID, "token ", snap)
			if stage == "success" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
func TestCompletionAndReadsValidateBeforeSQL(t *testing.T) {
	repo, _ := mockRepository(t)
	s := NewRunStore(repo.db)
	require.ErrorIs(t, s.CompleteScan(context.Background(), "", "", port.SignalSnapshot{}), port.ErrInvalidPortValue)
	require.ErrorIs(t, s.CompleteBacktest(context.Background(), "", "", backtest.Result{}), port.ErrInvalidPortValue)
	_, err := s.Orders(context.Background(), "run", port.PageRequest{})
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	_, err = NewSignalSnapshotStore(repo.db).Latest(context.Background(), port.SnapshotKey{}, port.PageRequest{Limit: 1})
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
}
func TestOutboxCopiesOpaquePayloadAndDeduplicates(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewOutbox(repo.db)
	event := port.Event{ID: "Event ", AggregateID: "Run A ", Kind: "test", Payload: []byte{0, 255, 1}, OccurredAt: testSnapshot().Key.AsOf}
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO `t_outbox_events`.*ON DUPLICATE KEY UPDATE").WillReturnResult(sqlmock.NewResult(1, 1))
	m.ExpectQuery("SELECT .*t_outbox_events").WillReturnRows(sqlmock.NewRows([]string{"event_id", "kind", "aggregate_id", "payload", "occurred_at"}).AddRow(event.ID, event.Kind, event.AggregateID, []byte(`"AP8B"`), event.OccurredAt))
	m.ExpectCommit()
	require.NoError(t, s.Publish(context.Background(), event))
}
