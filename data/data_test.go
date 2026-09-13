package data

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	mysqlgorm "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
	"trading/config"
)

func TestMySQLDSNUsesUTCForKernelDatetimeRoundTrips(t *testing.T) {
	dsn := mysqlDSN(config.DB{User: "user", Password: "password", Host: "127.0.0.1", Port: 3306, DBName: "trading"})
	parsed, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, time.UTC, parsed.Loc)
	require.True(t, parsed.ParseTime)
	input := time.Date(2026, 1, 2, 15, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	// DATETIME has no timezone: driver formatting and parsing must agree on UTC.
	wire := input.In(parsed.Loc).Format("2006-01-02 15:04:05")
	require.Equal(t, "2026-01-02 07:00:00", wire)
	decoded, err := time.ParseInLocation("2006-01-02 15:04:05", wire, parsed.Loc)
	require.NoError(t, err)
	require.True(t, input.Equal(decoded))
}

func TestInitializationClosesConnectionOnLegacyMigrationFailure(t *testing.T) {
	conn, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	db, err := gorm.Open(mysqlgorm.New(mysqlgorm.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("test"))
	mock.ExpectQuery("SELECT SCHEMA_NAME").WillReturnRows(sqlmock.NewRows([]string{"schema_name"}).AddRow("test"))
	mock.ExpectQuery("SELECT count.*information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	failure := errors.New("legacy migration unavailable")
	mock.ExpectExec("CREATE TABLE `t_stock_kline_daily`").WillReturnError(failure)
	mock.ExpectClose()
	got, err := initializeData(config.DB{}, db, migrateSchema)
	require.Nil(t, got)
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, conn.Ping(), "database is closed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitializationTransfersConnectionOnlyOnSuccess(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "kernel failure"}[fail], func(t *testing.T) {
			conn, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			require.NoError(t, err)
			db, err := gorm.Open(mysqlgorm.New(mysqlgorm.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			if fail {
				mock.ExpectClose()
			} else {
				mock.ExpectPing()
				mock.ExpectClose()
			}
			got, err := initializeData(config.DB{}, db, func(*gorm.DB) error {
				if fail {
					return errors.New("kernel migration failed")
				}
				return nil
			})
			if fail {
				require.Nil(t, got)
				require.Error(t, err)
				require.ErrorContains(t, conn.Ping(), "database is closed")
			} else {
				require.NoError(t, err)
				require.Same(t, db, got.DB())
				require.NoError(t, conn.Ping())
				require.NoError(t, conn.Close())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
