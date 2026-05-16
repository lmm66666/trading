package filter

import (
	"trading/model"
	"trading/pkg/indicator"
)

// SupportHoldFilter 价格站稳支撑位的过滤器
type SupportHoldFilter struct {
	MAPeriod int // 均线周期，如 20
}

func NewSupportHoldFilter(maPeriod int) *SupportHoldFilter {
	return &SupportHoldFilter{MAPeriod: maPeriod}
}

func (f *SupportHoldFilter) Filter(klines []*model.StockKline) []Result {
	n := len(klines)
	if n == 0 {
		return nil
	}

	prices := make([]float64, n)
	for i, k := range klines {
		prices[i] = k.Close
	}

	maResults := indicator.ComputeMA(prices, f.MAPeriod)
	results := make([]Result, n)

	for i := range n {
		valid := false
		if maResults[i] > 0 {
			// 收盘价在 MA 上方或小幅跌破（不超过 2%）
			valid = klines[i].Close >= maResults[i]*0.98
		}
		results[i] = Result{
			Date:  klines[i].Date,
			Valid: valid,
		}
	}
	return results
}
