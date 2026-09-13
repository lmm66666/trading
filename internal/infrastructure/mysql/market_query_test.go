package mysql

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func TestVisibilitySQLUsesExclusiveUpperBoundary(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "test:test@tcp(localhost:3306)/test", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	stmt := visibleAt(db, 3).Find(&[]MarketBarModel{}).Statement
	require.Contains(t, stmt.SQL.String(), "valid_from_version <= ? AND (valid_to_version IS NULL OR valid_to_version > ?)")
	require.Equal(t, []any{uint64(3), uint64(3)}, stmt.Vars)
}

func TestLookbackQueriesBoundRowsAndPreparedArguments(t *testing.T) {
	ids := make([]uint64, 5000)
	for i := range ids {
		ids[i] = uint64(i + 1)
	}
	req := port.BatchRequest{PrimaryTimeframe: market.Day, Auxiliary: []market.Timeframe{market.Week, market.Month}, From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Version: 7, LookbackBars: 20}
	queries := lookbackQueries(ids, req)
	require.Len(t, queries, 2)
	for _, q := range queries {
		require.LessOrEqual(t, len(q.args), 65535)
		require.Contains(t, q.sql, "close_time < ?")
		require.Contains(t, q.sql, "ORDER BY close_time DESC LIMIT ?")
		require.Contains(t, q.sql, "valid_to_version > ?")
		require.Equal(t, len(q.args), strings.Count(q.sql, "?"))
		require.NotContains(t, q.sql, fmt.Sprint(req.From))
	}
	req.LookbackBars = 0
	require.Empty(t, lookbackQueries(ids, req))
	req.LookbackBars = 20
	require.Empty(t, lookbackQueries(nil, req))
}

func TestWindowBarsKeepsAtMostRequestedWarmup(t *testing.T) {
	from := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	bars := make([]market.Bar, 5)
	for i := range bars {
		bars[i].CloseTime = from.AddDate(0, 0, i-3)
	}
	require.Len(t, windowBars(bars, from, 0), 2)
	require.Len(t, windowBars(bars, from, 2), 4)
	require.Len(t, windowBars(bars, from, 20), 5)
	require.Empty(t, windowBars(nil, from, 2))
}
