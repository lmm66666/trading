package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
	"trading/internal/port"
)

func TestQueueErrorsAndValidation(t *testing.T) {
	for _, stage := range []string{"begin", "update", "read", "deadlock"} {
		t.Run(stage, func(t *testing.T) {
			repo, m := mockRepository(t)
			q := NewJobQueue(repo.db)
			injected := errors.New("database offline")
			if stage == "begin" {
				m.ExpectBegin().WillReturnError(injected)
			} else {
				m.ExpectBegin()
				if stage == "deadlock" {
					m.ExpectExec("UPDATE t_compute_runs").WillReturnError(&driver.MySQLError{Number: 1213})
					m.ExpectRollback()
					m.ExpectBegin()
				}
				update := m.ExpectExec("UPDATE t_compute_runs")
				if stage == "update" {
					update.WillReturnError(injected)
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					m.ExpectQuery("SELECT .*t_compute_runs").WillReturnError(injected)
				}
				m.ExpectRollback()
			}
			_, err := q.Claim(context.Background(), "worker", time.Second)
			require.ErrorIs(t, err, injected)
		})
	}
	repo, _ := mockRepository(t)
	q := NewJobQueue(repo.db)
	ctx := context.Background()
	invalids := []port.Run{{}, queuedRun(), queuedRun(), queuedRun()}
	invalids[1].Status = port.RunSucceeded
	invalids[2].Attempts = 1
	now := time.Now().UTC()
	invalids[3].CancelRequestedAt = &now
	for _, run := range invalids {
		_, err := q.Enqueue(ctx, run)
		require.ErrorIs(t, err, port.ErrInvalidPortValue)
	}
	for _, id := range []string{"", strings.Repeat("x", 65)} {
		require.ErrorIs(t, q.Renew(ctx, id, "token", time.Second), port.ErrInvalidPortValue)
		require.ErrorIs(t, q.Retry(ctx, id, "token", now, port.Failure{}), port.ErrInvalidPortValue)
		require.ErrorIs(t, q.RequestCancel(ctx, id), port.ErrInvalidPortValue)
	}
	require.ErrorIs(t, q.Renew(ctx, "run", "", time.Second), port.ErrInvalidPortValue)
	require.ErrorIs(t, q.Renew(ctx, "run", "token", 0), port.ErrInvalidPortValue)
	require.ErrorIs(t, q.Retry(ctx, "run", "token", now, port.Failure{}), port.ErrInvalidPortValue)
	require.ErrorIs(t, q.Retry(ctx, "run", "token", time.Time{}, port.Failure{Code: "E", Message: "failure"}), port.ErrInvalidPortValue)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := q.Claim(cancelled, "owner", time.Second)
	require.ErrorIs(t, err, context.Canceled)
}
func TestRetryAndRenewCAS(t *testing.T) {
	for _, operation := range []string{"renew", "retry", "last", "permanent"} {
		for _, affected := range []int64{0, 1} {
			t.Run(operation+string(rune('0'+affected)), func(t *testing.T) {
				repo, m := mockRepository(t)
				q := NewJobQueue(repo.db)
				m.ExpectBegin()
				rows := runRows(port.RunRunning)
				if operation == "last" {
					rows = sqlmock.NewRows([]string{"run_id", "lease_owner", "lease_token", "attempts"}).AddRow("Run A ", "owner ", "token ", 4)
				}
				m.ExpectQuery("SELECT .*t_compute_runs.*FOR UPDATE").WillReturnRows(rows)
				m.ExpectExec("UPDATE `t_compute_runs`.*WHERE run_id = .*lease_owner = .*lease_token = .*lease_until > UTC_TIMESTAMP\\(6\\)").WillReturnResult(sqlmock.NewResult(0, affected))
				if affected == 0 {
					m.ExpectRollback()
				} else {
					m.ExpectCommit()
				}
				var err error
				if operation == "renew" {
					err = q.Renew(context.Background(), "Run A ", "token ", time.Minute)
				} else {
					err = q.Retry(context.Background(), "Run A ", "token ", time.Now().UTC(), port.Failure{Code: "TEMP", Message: "temporary", Retryable: operation != "permanent"})
				}
				if affected == 0 {
					require.ErrorIs(t, err, port.ErrLeaseLost)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}
func TestCancelAndReaper(t *testing.T) {
	for _, status := range []port.RunStatus{port.RunPending, port.RunRunning, port.RunSucceeded} {
		t.Run(string(status), func(t *testing.T) {
			repo, m := mockRepository(t)
			m.ExpectBegin()
			m.ExpectQuery("SELECT .*t_compute_runs.*FOR UPDATE").WillReturnRows(runRows(status))
			if status != port.RunSucceeded {
				m.ExpectExec("UPDATE `t_compute_runs`.*WHERE run_id = .*status = .*lease_owner = .*lease_token =").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			m.ExpectCommit()
			require.NoError(t, NewJobQueue(repo.db).RequestCancel(context.Background(), "Run A "))
		})
	}
	for _, fail := range []bool{false, true} {
		repo, m := mockRepository(t)
		m.ExpectBegin()
		query := m.ExpectQuery("SELECT .*t_compute_runs.*attempts >= 4.*FOR UPDATE")
		if fail {
			query.WillReturnError(errors.New("db failed"))
			m.ExpectRollback()
		} else {
			query.WillReturnRows(runRows(port.RunRunning))
			m.ExpectExec("UPDATE `t_compute_runs`.*lease_owner = .*lease_token = .*attempts >= 4.*lease_until <= UTC_TIMESTAMP\\(6\\)").WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
		}
		count, err := NewJobQueue(repo.db).ReapExpired(context.Background())
		if fail {
			require.Error(t, err)
			require.Zero(t, count)
		} else {
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
		}
	}
}
func TestEnqueueAndCancelDatabaseErrors(t *testing.T) {
	for _, stage := range []string{"insert", "read", "conflict", "commit"} {
		t.Run(stage, func(t *testing.T) {
			repo, m := mockRepository(t)
			m.ExpectBegin()
			e := m.ExpectExec("INSERT INTO `t_compute_runs`")
			if stage == "insert" {
				e.WillReturnError(errors.New("insert"))
			} else {
				e.WillReturnResult(sqlmock.NewResult(1, 1))
				read := m.ExpectQuery("SELECT .*t_compute_runs")
				if stage == "read" {
					read.WillReturnError(errors.New("read"))
				} else {
					rows := runRows(port.RunPending)
					if stage == "conflict" {
						rows = sqlmock.NewRows([]string{"input_hash"}).AddRow("different")
					}
					read.WillReturnRows(rows)
				}
			}
			if stage == "commit" {
				m.ExpectCommit().WillReturnError(errors.New("commit"))
			} else {
				m.ExpectRollback()
			}
			got, err := NewJobQueue(repo.db).Enqueue(context.Background(), queuedRun())
			require.Error(t, err)
			require.Empty(t, got.ID)
		})
	}
	for _, err := range []error{gorm.ErrRecordNotFound, errors.New("storage")} {
		repo, m := mockRepository(t)
		m.ExpectBegin()
		m.ExpectQuery("SELECT .*t_compute_runs").WillReturnError(err)
		m.ExpectRollback()
		require.Error(t, NewJobQueue(repo.db).RequestCancel(context.Background(), "run"))
	}
}
