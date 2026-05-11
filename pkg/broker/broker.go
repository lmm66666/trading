package broker

import (
	"context"

	"trading/model"
)

// Broker 定义行情数据提供者的统一接口
type Broker interface {
	// GetStockTodayInBatch 批量获取今日行情数据
	// codes: 代码列表，如 ["sh000001"]
	// 返回: map[code] => StockKline
	GetStockTodayInBatch(ctx context.Context, codes []string) (map[string]*model.StockKline, error)

	// GetStockToday 获取单个代码的今日数据
	GetStockToday(ctx context.Context, code string) (*model.StockKline, error)

	// GetStockHistorical 获取历史 K 线数据
	// symbol: 股票代码（如 sh000001）
	// scale: 时间粒度（分钟，如 5, 15, 30, 60, 240）
	// length: 数据条数
	// 返回: StockKline 数组（按日期升序）
	GetStockHistorical(ctx context.Context, symbol string, scale int, length int) ([]model.StockKline, error)

	// GetFinancialReportHistorical 获取历史财报数据
	// symbol: 股票代码（如 sh600004）
	// page: 页码，从 1 开始
	// num: 每页条数
	// 返回: 财报列表、总条数
	GetFinancialReportHistorical(ctx context.Context, symbol string, page, num int) ([]*model.FinancialReport, int, error)
}
