package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/port"
)

func TestResolveCodeQueriesOnlyExactActiveInstrumentsAndBoundsAmbiguity(t *testing.T) {
	repo, m := mockRepository(t)
	m.ExpectQuery("SELECT .*t_instruments.*code = .*active = .*ORDER BY exchange.*LIMIT").WithArgs("600000", true, 2).WillReturnRows(sqlmock.NewRows([]string{"exchange", "code"}).AddRow("SSE", "600000").AddRow("BSE", "600000"))
	ids, err := repo.ResolveCode(context.Background(), "600000")
	require.NoError(t, err)
	require.Len(t, ids, 2)
	require.Equal(t, "SSE:600000", ids[0].String())
	for _, code := range []string{"60000", "600000 ", "abcdef", "' OR 1"} {
		_, err := repo.ResolveCode(context.Background(), code)
		require.ErrorIs(t, err, port.ErrInvalidPortValue)
	}
}
func TestLatestPublishedKeyPinsSnapshotAndPreservesIdentity(t *testing.T) {
	repo, m := mockRepository(t)
	s := NewSignalSnapshotStore(repo.db)
	for _, snapshot := range []string{"", "Snapshot A "} {
		pattern := "SELECT .*t_signal_snapshots.*strategy_id = .*status IN .*"
		if snapshot != "" {
			pattern += "snapshot_id = .*"
		}
		pattern += "ORDER BY as_of DESC, data_version DESC, id DESC.*LIMIT"
		q := m.ExpectQuery(pattern)
		if snapshot == "" {
			q.WithArgs("daily_b1_buy", "SUCCEEDED", "PARTIAL_SUCCEEDED", 1)
		} else {
			q.WithArgs("daily_b1_buy", "SUCCEEDED", "PARTIAL_SUCCEEDED", snapshot, 1)
		}
		q.WillReturnRows(sqlmock.NewRows([]string{"snapshot_id", "strategy_id", "strategy_version", "parameters_hash", "as_of"}).AddRow("Snapshot A ", "daily_b1_buy", "1", "Hash A ", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
		key, err := s.LatestPublishedKey(context.Background(), "daily_b1_buy", "", snapshot)
		require.NoError(t, err)
		require.Equal(t, "Snapshot A ", key.SnapshotID)
		require.Equal(t, "Hash A ", key.ParametersHash)
	}
}
func TestAPIReadersErrorsAndVersionFilters(t *testing.T) {
	repo, m := mockRepository(t)
	m.ExpectQuery("SELECT .*t_instruments").WillReturnError(errors.New("database"))
	_, err := repo.ResolveCode(context.Background(), "600000")
	require.Error(t, err)
	s := NewSignalSnapshotStore(repo.db)
	_, err = s.LatestPublishedKey(context.Background(), "", "", "")
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	m.ExpectQuery("SELECT .*t_signal_snapshots.*strategy_version =").WithArgs("strategy", "SUCCEEDED", "PARTIAL_SUCCEEDED", "1", 1).WillReturnRows(emptyRows())
	_, err = s.LatestPublishedKey(context.Background(), "strategy", "1", "")
	require.ErrorIs(t, err, port.ErrSnapshotNotReady)
	m.ExpectQuery("SELECT .*t_signal_snapshots").WillReturnError(errors.New("database"))
	_, err = s.LatestPublishedKey(context.Background(), "strategy", "", "")
	require.Error(t, err)
}
