package mysql

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"trading/internal/market"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 检查实际生成 SQL 的绑定数，不依赖实现中的 schema 计数或分块函数。
func mockLegacyParameterBudget(t *testing.T, table string, columns int) (*gorm.DB, sqlmock.Sqlmock, *[]int) {
	t.Helper()
	counts := []int{}
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if expected != "bounded insert" {
			return sqlmock.QueryMatcherRegexp.Match(expected, actual)
		}
		count := strings.Count(actual, "?")
		if !strings.HasPrefix(actual, "INSERT INTO `"+table+"`") || count > 65535-1024 || count == 0 || count%columns != 0 {
			return fmt.Errorf("unexpected insert binding count: %d", count)
		}
		counts = append(counts, count/columns)
		return nil
	})
	conn, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	db, err := gorm.Open(driver.New(driver.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = conn.Close() })
	return db, mock, &counts
}

func TestLegacyLargeTargetBatchSplitsAtomicallyAndRetriesWholeFailedBatch(t *testing.T) {
	for _, tc := range []struct {
		name, table   string
		kind, columns int
	}{
		{"daily", "t_market_bars", 0, 18},
		{"weekly", "t_market_bars", 1, 18},
		{"factors", "t_adjustment_factors", 2, 9},
		{"actions", "t_corporate_actions", 3, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, counts := mockLegacyParameterBudget(t, tc.table, tc.columns)
			item := longLegacyFixture(10000)
			batch, err := backfillLegacy(item)
			require.NoError(t, err)
			switch tc.kind {
			case 1:
				batch.Bars[market.Week] = batch.Bars[market.Day]
				delete(batch.Bars, market.Day)
			case 2:
				batch.Bars = map[market.Timeframe][]market.Bar{}
				batch.Factors = make([]market.AdjustmentFactor, 10000)
			case 3:
				batch.Bars = map[market.Timeframe][]market.Bar{}
				batch.Actions = make([]market.CorporateAction, 10000)
			}
			state := legacyTargetCursor{InstrumentID: 41, Digest: batch.Digest}
			failure := errors.New("internal chunk failed")
			mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, state))
			mock.ExpectBegin()
			mock.ExpectExec("bounded insert").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("bounded insert").WillReturnError(failure)
			mock.ExpectRollback() // 不允许游标前进或提交第一段。
			require.ErrorIs(t, writeLegacyInstrument(db, 1, item, batch, 10000), failure)
			require.Len(t, *counts, 2)
			firstChunk := (*counts)[0]
			*counts = nil

			// 恢复仍从外部批次起点读取检查点，重新写完整的 10000 行。
			mock.ExpectQuery("SELECT .*t_legacy_kernel_migration").WillReturnRows(targetStateRows(t, state))
			mock.ExpectBegin()
			chunks := 2
			if tc.kind < 2 {
				chunks = 3
			}
			for i := 0; i < chunks; i++ {
				mock.ExpectExec("bounded insert").WillReturnResult(sqlmock.NewResult(int64(i+1), 1))
			}
			positions := [4]int{}
			positions[tc.kind] = 10000
			expectTargetCheckpoint(mock, item.ID, positions, false)
			mock.ExpectCommit() // 所有内部 INSERT 后才允许提交检查点。
			verification := errors.New("verification boundary")
			mock.ExpectQuery("SELECT .*t_market_bars").WillReturnError(verification)
			require.ErrorIs(t, writeLegacyInstrument(db, 1, item, batch, 10000), verification)
			require.Len(t, *counts, chunks)
			require.Equal(t, firstChunk, (*counts)[0])
			total := 0
			for _, n := range *counts {
				total += n
			}
			require.Equal(t, 10000, total)
		})
	}
}
