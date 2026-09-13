package mysql

import (
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
	"strings"
	"sync"
	"testing"
	"trading/internal/backtest"
)

// These columns participate in byte-exact identity, equality or lookup. Finite
// enums and display strings intentionally remain ordinary character columns.
var exactIdentityModels = []struct {
	model  any
	fields map[string]int
}{
	{&InstrumentModel{}, map[string]int{"Source": 128}},
	{&DataVersionModel{}, map[string]int{"Source": 128, "Digest": 64}},
	{&CorporateActionModel{}, map[string]int{"SourceEventID": 128}},
	{&ComputeRunModel{}, map[string]int{"RunID": 64, "IdempotencyKey": 128, "InputHash": 64, "StrategyID": 64, "StrategyVersion": 32, "EngineVersion": 32, "LeaseOwner": 128, "LeaseToken": 128}},
	{&BacktestRunModel{}, map[string]int{"RunID": 64, "StrategyID": 64, "StrategyVersion": 32, "EngineVersion": 32}},
	{&BacktestOrderModel{}, map[string]int{"RunID": 64}},
	{&BacktestTradeModel{}, map[string]int{"RunID": 64}},
	{&BacktestEquityModel{}, map[string]int{"RunID": 64}},
	{&SignalSnapshotModel{}, map[string]int{"SnapshotID": 64, "RunID": 64, "StrategyID": 64, "StrategyVersion": 32, "ParametersHash": 64}},
	{&SignalSnapshotRowModel{}, map[string]int{"SnapshotID": 64}},
	{&OutboxEventModel{}, map[string]int{"EventID": 64, "AggregateID": 128}},
}

func TestOpaqueIdentitySchemaPreservesCaseAndTrailingBytes(t *testing.T) {
	for _, entry := range exactIdentityModels {
		s, err := schema.Parse(entry.model, &sync.Map{}, schema.NamingStrategy{})
		require.NoError(t, err)
		for name, size := range entry.fields {
			field := s.FieldsByName[name]
			require.NotNil(t, field)
			require.Equal(t, fmt.Sprintf("varbinary(%d)", size), string(field.DataType), s.Table+"."+name)
		}
	}
	for _, model := range []any{&BacktestOrderModel{}, &BacktestTradeModel{}} {
		s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
		require.NoError(t, err)
		for _, name := range []string{"OrderID", "FillID"} {
			if field := s.FieldsByName[name]; field != nil {
				require.Equal(t, "longblob", string(field.DataType))
			}
		}
	}
}

func TestUnboundedOrderAndFillIdentityMappingKeepsExactBytes(t *testing.T) {
	repo, mock := mockRepository(t)
	for _, id := range []string{"Key", "key", "key ", "成交标识 ", strings.Repeat("长", 500)} {
		// Actual GORM row decoding is exercised here; typed IDs remain strings
		// when later RunStore maps the model back into the domain.
		mock.ExpectQuery("SELECT .*t_backtest_orders").WillReturnRows(sqlmock.NewRows([]string{"order_id"}).AddRow([]byte(id)))
		var row BacktestOrderModel
		require.NoError(t, repo.db.Take(&row).Error)
		order := backtest.Order{ID: row.OrderID}
		require.Equal(t, []byte(id), []byte(order.ID))
		mock.ExpectQuery("SELECT .*t_backtest_trades").WillReturnRows(sqlmock.NewRows([]string{"fill_id", "order_id"}).AddRow([]byte(id), []byte(id)))
		var trade BacktestTradeModel
		require.NoError(t, repo.db.Take(&trade).Error)
		fill := backtest.Fill{ID: trade.FillID, OrderID: trade.OrderID}
		require.Equal(t, id, fill.ID)
		require.Equal(t, id, fill.OrderID)
		// Verify emitted SQL parameters too: the domain ID must reach the
		// binary column without trimming, case-folding or a 128/512-byte cap.
		mock.ExpectExec("INSERT INTO `t_backtest_orders`").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "run", uint64(1), id, sqlmock.AnyArg(), nil, "", "", int64(0), int64(0), int64(0), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
		require.NoError(t, repo.db.Create(&BacktestOrderModel{RunID: "run", Sequence: 1, OrderID: order.ID}).Error)
		mock.ExpectExec("INSERT INTO `t_backtest_trades`").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "run", uint64(1), uint64(0), id, id, sqlmock.AnyArg(), "", int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)).WillReturnResult(sqlmock.NewResult(1, 1))
		require.NoError(t, repo.db.Create(&BacktestTradeModel{RunID: "run", Sequence: 1, FillID: fill.ID, OrderID: fill.OrderID}).Error)
	}
}
