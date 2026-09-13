package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
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

func TestSearchInstrumentsRanksExactCodeAndReturnsMetadata(t *testing.T) {
	repo, m := mockRepository(t)
	m.ExpectQuery("SELECT .*exchange.*code.*name.*board.*active.*lot_size.*t_instruments.*active = .*exchange = .*code LIKE .*ORDER BY CASE WHEN code = .*exchange, code LIMIT").
		WithArgs(true, market.SZSE, "002%", "002", 20).
		WillReturnRows(sqlmock.NewRows([]string{"exchange", "code", "name", "board", "active", "lot_size"}).
			AddRow("SZSE", "002415", "海康威视", "MAIN", true, 100))

	items, err := repo.Search(context.Background(), port.InstrumentSearch{Query: "002", Exchange: market.SZSE, Limit: 20})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "SZSE:002415", items[0].ID.String())
	require.Equal(t, "海康威视", items[0].Name)
	require.Equal(t, int64(100), items[0].LotSize)
}

func TestSearchInstrumentsEscapesNameWildcards(t *testing.T) {
	repo, m := mockRepository(t)
	m.ExpectQuery("SELECT .*t_instruments.*active = .*name LIKE .*ORDER BY CASE WHEN name = .*WHEN name LIKE .*exchange, code LIMIT").
		WithArgs(true, `%A\%\_B%`, `A%_B`, `A\%\_B%`, 10).
		WillReturnRows(sqlmock.NewRows([]string{"exchange", "code", "name", "board", "active", "lot_size"}))

	items, err := repo.Search(context.Background(), port.InstrumentSearch{Query: "A%_B", Limit: 10})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestGetInstrumentReturnsNotFoundForInactiveOrMissingInstrument(t *testing.T) {
	repo, m := mockRepository(t)
	m.ExpectQuery("SELECT .*t_instruments.*exchange = .*code = .*active = .*LIMIT").
		WithArgs(market.SSE, "600000", true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"exchange", "code", "name", "board", "active", "lot_size"}))

	_, err := repo.Get(context.Background(), market.InstrumentID{Exchange: market.SSE, Code: "600000"})
	require.ErrorIs(t, err, port.ErrMarketDataNotFound)
}
