package mysql

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"trading/internal/port"
)

func queuedRun() port.Run {
	return port.Run{ID: "Run A ", IdempotencyKey: "Key ", InputHash: "hash", Kind: port.RunScan, Status: port.RunPending, StrategyID: "strategy", StrategyVersion: "v1", EngineVersion: "v1", DataVersion: 1, RequestJSON: []byte(`{}`)}
}
func runRows(status port.RunStatus) *sqlmock.Rows {
	owner, token := "", ""
	if status == port.RunRunning {
		owner, token = "owner ", "token "
	}
	return sqlmock.NewRows([]string{"id", "run_id", "kind", "status", "idempotency_key", "input_hash", "strategy_id", "strategy_version", "engine_version", "data_version", "request_json", "lease_owner", "lease_token", "attempts"}).AddRow(1, "Run A ", "SCAN", string(status), "Key ", "hash", "strategy", "v1", "v1", 1, []byte(`{}`), owner, token, 1)
}
func TestClaimUsesAtomicOrderedDatabaseLease(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	m.ExpectBegin()
	m.ExpectExec(fmt.Sprintf(`UPDATE t_compute_runs SET .*UTC_TIMESTAMP\(6\).*attempts < %d.*ORDER BY created_at, id LIMIT 1`, port.MaxRunAttempts)).WithArgs("owner ", sqlmock.AnyArg(), int64(60000000)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("SELECT .*t_compute_runs.*lease_owner = .*lease_token =").WillReturnRows(runRows(port.RunRunning))
	m.ExpectCommit()
	run, err := q.Claim(context.Background(), "owner ", time.Minute)
	require.NoError(t, err)
	require.Equal(t, "Run A ", run.ID)
	require.Equal(t, port.RunRunning, run.Status)
}
func TestClaimNeverReturnsLeaseAfterCommitFailure(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	m.ExpectBegin()
	m.ExpectExec("UPDATE t_compute_runs").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("SELECT .*t_compute_runs").WillReturnRows(runRows(port.RunRunning))
	m.ExpectCommit().WillReturnError(errors.New("commit unknown"))
	run, err := q.Claim(context.Background(), "owner", time.Second)
	require.Error(t, err)
	require.Empty(t, run.ID)
}
func TestClaimEmptyAndInvalid(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	_, err := q.Claim(context.Background(), "", time.Second)
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	_, err = q.Claim(context.Background(), "owner", time.Nanosecond)
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	m.ExpectBegin()
	m.ExpectExec("UPDATE t_compute_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectRollback()
	_, err = q.Claim(context.Background(), "owner", time.Second)
	require.ErrorIs(t, err, port.ErrRunNotFound)
}
func TestEnqueueDuplicateReturnsPersistedRun(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO `t_compute_runs`.*ON DUPLICATE KEY UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT .*t_compute_runs.*kind = .*idempotency_key =").WithArgs("SCAN", "Key ", 1).WillReturnRows(runRows(port.RunPending))
	m.ExpectCommit()
	input := queuedRun()
	input.ID = "replacement"
	got, err := q.Enqueue(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Run A ", got.ID)
}
func TestRenewRejectsStaleOrExpiredLease(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	m.ExpectBegin()
	m.ExpectQuery("SELECT .*t_compute_runs.*run_id = .*lease_token = .*lease_until > UTC_TIMESTAMP\\(6\\).*FOR UPDATE").WithArgs("Run A ", "token ", 1).WillReturnRows(emptyRows())
	m.ExpectRollback()
	require.ErrorIs(t, q.Renew(context.Background(), "Run A ", "token ", time.Minute), port.ErrLeaseLost)
}

func TestEnqueueReportsExplicitIdempotencyCollision(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO `t_compute_runs`.*ON DUPLICATE KEY UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT .*t_compute_runs.*kind = .*idempotency_key =").WithArgs("SCAN", "Key ", 1).WillReturnRows(runRows(port.RunPending))
	m.ExpectRollback()
	input := queuedRun()
	input.InputHash = "different"
	_, err := q.Enqueue(context.Background(), input)
	require.ErrorIs(t, err, port.ErrIdempotencyConflict)
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
}
