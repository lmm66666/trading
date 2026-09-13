package broker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"trading/model"
)

var (
	ErrUpstream          = errors.New("broker: upstream failure")
	ErrUpstreamTimeout   = errors.New("broker: upstream timeout")
	ErrRequestCanceled   = errors.New("broker: request canceled")
	ErrInvalidRequest    = errors.New("broker: invalid request")
	ErrMalformedResponse = errors.New("broker: malformed upstream response")
	ErrIncompleteData    = errors.New("broker: incomplete market data")
)

// UpstreamError carries safe transport metadata. It deliberately excludes the
// request URL and response body so callers can surface it without leaking data.
type UpstreamError struct {
	Kind          error
	Cause         error
	StatusCode    int
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *UpstreamError) Error() string {
	message := safeUpstreamKind(e.Kind)
	if e.StatusCode != 0 {
		message = fmt.Sprintf("%s: status=%d", message, e.StatusCode)
	}
	if e.HasRetryAfter {
		message = fmt.Sprintf("%s: retry_after=%s", message, e.RetryAfter)
	}
	return message
}

func safeUpstreamKind(kind error) string {
	switch {
	case errors.Is(kind, ErrUpstreamTimeout):
		return ErrUpstreamTimeout.Error()
	case errors.Is(kind, ErrRequestCanceled):
		return ErrRequestCanceled.Error()
	default:
		return ErrUpstream.Error()
	}
}

func (e *UpstreamError) Unwrap() error {
	if e.Cause == nil {
		return e.Kind
	}
	return errors.Join(e.Kind, e.Cause)
}

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
