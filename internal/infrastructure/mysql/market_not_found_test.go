package mysql

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestMarketLatestEmptyUsesPortError(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery("SELECT .*t_market_data_versions").WillReturnRows(sqlmock.NewRows([]string{"version"}))
	_, err := repo.LatestCompleteVersion(context.Background())
	require.ErrorIs(t, err, port.ErrMarketDataNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
