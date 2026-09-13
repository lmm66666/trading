package mysql

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestSnapshotContinuationRequiresExactIdentity(t *testing.T) {
	repo, _ := mockRepository(t)
	got, err := NewSignalSnapshotStore(repo.db).Latest(context.Background(), testSnapshot().Key, port.PageRequest{AfterSequence: 1, Limit: 1})
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
	require.Empty(t, got.ID)
}

func TestSnapshotPaginationRetainsOriginalIDAfterLatestVersionChanges(t *testing.T) {
	repo, m := mockRepository(t)
	store := NewSignalSnapshotStore(repo.db)
	key := testSnapshot().Key
	page := port.PageRequest{Limit: 1}
	for index, selection := range []struct {
		id, code string
		version  int
	}{{"snapshot", "600000", 1}, {"new", "600002", 2}, {"snapshot", "600001", 1}} {
		metadata := m.ExpectQuery("SELECT .*t_signal_snapshots")
		if index == 2 {
			metadata.WithArgs("strategy", "v1", "hash", "SUCCEEDED", "PARTIAL_SUCCEEDED", key.AsOf, "snapshot", 1)
		} else {
			metadata.WithArgs("strategy", "v1", "hash", "SUCCEEDED", "PARTIAL_SUCCEEDED", key.AsOf, 1)
		}
		metadata.WillReturnRows(sqlmock.NewRows([]string{"snapshot_id", "run_id", "strategy_id", "strategy_version", "parameters_hash", "data_version", "as_of", "failures_json"}).AddRow(selection.id, "run", "strategy", "v1", "hash", selection.version, key.AsOf, []byte(`[]`)))
		cursor := int64(0)
		if index == 2 {
			cursor = 1
		}
		m.ExpectQuery("SELECT r.*, i.exchange, i.code").WithArgs(selection.id, cursor, 1).WillReturnRows(sqlmock.NewRows([]string{"exchange", "code", "signal_time", "reason", "values_json"}).AddRow("SSE", selection.code, key.AsOf, "hit", []byte(`{}`)))
	}
	first, err := store.Latest(context.Background(), key, page)
	require.NoError(t, err)
	newest, err := store.Latest(context.Background(), key, page)
	require.NoError(t, err)
	require.Equal(t, "new", newest.ID)
	require.Equal(t, first.Key.AsOf, newest.Key.AsOf)
	require.Greater(t, newest.DataVersion, first.DataVersion)
	second, err := store.Latest(context.Background(), first.Key, port.PageRequest{AfterSequence: 1, Limit: 1})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, []string{"600000", "600001"}, []string{first.Rows[0].Instrument.Code, second.Rows[0].Instrument.Code})
}

func TestSnapshotContinuationConstrainsIdentityAndBusinessKey(t *testing.T) {
	repo, m := mockRepository(t)
	store := NewSignalSnapshotStore(repo.db)
	key := testSnapshot().Key
	key.SnapshotID = "snapshot "
	m.ExpectQuery("SELECT .*t_signal_snapshots.*strategy_id = .*strategy_version = .*parameters_hash = .*status IN .*as_of = .*snapshot_id =").WithArgs("strategy", "v1", "hash", "SUCCEEDED", "PARTIAL_SUCCEEDED", key.AsOf, "snapshot ", 1).WillReturnRows(sqlmock.NewRows([]string{"snapshot_id", "run_id", "strategy_id", "strategy_version", "parameters_hash", "data_version", "as_of", "failures_json"}).AddRow("snapshot ", "old run", "strategy", "v1", "hash", 1, key.AsOf, []byte(`[]`)))
	m.ExpectQuery("SELECT r.*, i.exchange, i.code").WithArgs("snapshot ", int64(1), 1).WillReturnRows(sqlmock.NewRows([]string{"sequence", "exchange", "code", "signal_time", "reason", "values_json"}).AddRow(2, "SSE", "600001", key.AsOf, "old second row", []byte(`{}`)))
	got, err := store.Latest(context.Background(), key, port.PageRequest{AfterSequence: 1, Limit: 1})
	require.NoError(t, err)
	require.Equal(t, "snapshot ", got.ID)
	require.Equal(t, "snapshot ", got.Key.SnapshotID)
	require.Equal(t, "600001", got.Rows[0].Instrument.Code)
}

func TestSnapshotPinnedLookupDoesNotFallbackForUnavailableID(t *testing.T) {
	for _, id := range []string{"unknown", "unpublished", "snapshot"} {
		t.Run(id, func(t *testing.T) {
			repo, m := mockRepository(t)
			key := testSnapshot().Key
			key.SnapshotID = id
			m.ExpectQuery("SELECT .*t_signal_snapshots.*status IN .*snapshot_id =").WithArgs("strategy", "v1", "hash", "SUCCEEDED", "PARTIAL_SUCCEEDED", key.AsOf, id, 1).WillReturnRows(emptyRows())
			got, err := NewSignalSnapshotStore(repo.db).Latest(context.Background(), key, port.PageRequest{AfterSequence: 1, Limit: 1})
			require.ErrorIs(t, err, port.ErrSnapshotNotReady)
			require.Empty(t, got.ID)
		})
	}
}
