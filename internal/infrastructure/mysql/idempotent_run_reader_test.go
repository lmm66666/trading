package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestIdempotentLookupKeepsExactIdentityAndPinnedVersion(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	m.ExpectQuery("SELECT .*t_compute_runs.*kind = .*idempotency_key =").WithArgs("SCAN", "Key ", 1).WillReturnRows(runRows(port.RunPending))
	run, err := q.FindByIdempotency(context.Background(), port.RunScan, "Key ")
	require.NoError(t, err)
	require.Equal(t, "Key ", run.IdempotencyKey)
	require.Equal(t, uint64(1), uint64(run.DataVersion))
}
func TestIdempotentLookupValidatesAndPropagatesUnavailableStorage(t *testing.T) {
	repo, m := mockRepository(t)
	q := NewJobQueue(repo.db)
	_, err := q.FindByIdempotency(context.Background(), "BAD", "key")
	require.Error(t, err)
	_, err = q.FindByIdempotency(context.Background(), port.RunScan, "")
	require.Error(t, err)
	m.ExpectQuery("SELECT .*t_compute_runs").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err = q.FindByIdempotency(context.Background(), port.RunScan, "key")
	require.ErrorIs(t, err, port.ErrRunNotFound)
	failure := errors.New("storage unavailable")
	m.ExpectQuery("SELECT .*t_compute_runs").WillReturnError(failure)
	_, err = q.FindByIdempotency(context.Background(), port.RunScan, "key")
	require.ErrorIs(t, err, failure)
}
