package mysql

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSnapshotIndexUpgradeDropsOnlyLegacyBusinessUniqueness(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery("SELECT COUNT.*information_schema.statistics").WithArgs("t_signal_snapshots", "uq_snapshot_business").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectExec("ALTER TABLE `t_signal_snapshots` DROP INDEX `uq_snapshot_business`").WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, upgradeSnapshotBusinessIndex(repo.db))
	mock.ExpectQuery("SELECT COUNT.*information_schema.statistics").WithArgs("t_signal_snapshots", "uq_snapshot_business").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	require.NoError(t, upgradeSnapshotBusinessIndex(repo.db))
}
func TestSnapshotIndexUpgradePropagatesReadAndDDLFailures(t *testing.T) {
	repo, mock := mockRepository(t)
	failure := errors.New("unavailable")
	mock.ExpectQuery("SELECT COUNT.*information_schema.statistics").WillReturnError(failure)
	require.ErrorIs(t, upgradeSnapshotBusinessIndex(repo.db), failure)
	mock.ExpectQuery("SELECT COUNT.*information_schema.statistics").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectExec("ALTER TABLE").WillReturnError(failure)
	require.ErrorIs(t, upgradeSnapshotBusinessIndex(repo.db), failure)
}
