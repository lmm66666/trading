package data

import (
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
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
