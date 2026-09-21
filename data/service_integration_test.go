//go:build integration

package data

import (
	"net"
	"strconv"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	mysqlgorm "gorm.io/driver/mysql"
	"trading/config"
	"trading/internal/infrastructure/mysql/dbtest"
)

func TestMySQLWorkbenchConnectsWithoutMigratingAndReadsUpdaterSchema(t *testing.T) {
	db := dbtest.OpenIsolatedMySQL(t, dbtest.Target)
	parsed, err := driver.ParseDSN(db.Dialector.(*mysqlgorm.Dialector).Config.DSN)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(parsed.Addr)
	require.NoError(t, err)
	number, err := strconv.Atoi(port)
	require.NoError(t, err)
	cfg := config.DB{Host: host, Port: number, User: parsed.User, Password: parsed.Passwd, DBName: parsed.DBName}
	before, err := Open(cfg)
	require.NoError(t, err)
	var tables int64
	require.NoError(t, before.DB().Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error)
	require.Zero(t, tables, "workbench must not create tables")
	beforeSQL, err := before.DB().DB()
	require.NoError(t, err)
	require.NoError(t, beforeSQL.Close())
	updater, err := New(cfg)
	require.NoError(t, err)
	updaterSQL, err := updater.DB().DB()
	require.NoError(t, err)
	defer updaterSQL.Close()
	workbench, err := Open(cfg)
	require.NoError(t, err)
	workbenchSQL, err := workbench.DB().DB()
	require.NoError(t, err)
	defer workbenchSQL.Close()
	var versions int64
	require.NoError(t, workbench.DB().Raw("SELECT COUNT(*) FROM t_market_data_versions").Scan(&versions).Error)
	require.Equal(t, int64(1), versions, "workbench must see updater initialization without running it")
	// Stopping the workbench must not close the updater's independent connection.
	require.NoError(t, workbenchSQL.Close())
	require.NoError(t, updaterSQL.Ping())
}
