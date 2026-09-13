package mysql

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/market"
)

func TestMarketChangesClassifiesFactorRevisionsWithoutPerInstrumentQueries(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery("SELECT.*t_market_data_versions").WithArgs(uint64(2), "COMPLETE", 1).WillReturnRows(versionRows(2))
	mock.ExpectQuery("SELECT.*t_market_data_versions").WithArgs(uint64(1), "COMPLETE", 1).WillReturnRows(versionRows(1))
	mock.ExpectQuery("SELECT i.*").WillReturnRows(sqlmock.NewRows([]string{"exchange", "code"}).AddRow("SSE", "600000"))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"changed"}).AddRow(true))
	changes, err := repo.MarketChanges(context.Background(), 1, 2)
	require.NoError(t, err)
	require.True(t, changes.FactorsOrActionsChanged)
	require.Equal(t, []market.InstrumentID{{Exchange: market.SSE, Code: "600000"}}, changes.Dirty)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarketChangesRejectsUnknownVersionAndClassifiesReadFailures(t *testing.T) {
	repo, mock := mockRepository(t)
	_, err := repo.MarketChanges(context.Background(), 2, 1)
	require.Error(t, err)
	for _, fail := range []bool{false, true} {
		mock.ExpectQuery("SELECT.*t_market_data_versions").WillReturnRows(versionRows(2))
		mock.ExpectQuery("SELECT.*t_market_data_versions").WillReturnRows(versionRows(1))
		mock.ExpectQuery("SELECT i.*").WillReturnRows(sqlmock.NewRows([]string{"exchange", "code"}))
		query := mock.ExpectQuery("SELECT EXISTS")
		if fail {
			query.WillReturnError(errors.New("connection"))
		} else {
			query.WillReturnRows(sqlmock.NewRows([]string{"changed"}).AddRow(false))
		}
		changes, err := repo.MarketChanges(context.Background(), 1, 2)
		if fail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.False(t, changes.FactorsOrActionsChanged)
			require.Empty(t, changes.Dirty)
		}
	}
}
