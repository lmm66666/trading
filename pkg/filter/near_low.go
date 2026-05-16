package filter

import (
	"trading/model"
)

// NearLowFilter 价格在近期低点附近的过滤器
type NearLowFilter struct {
	Period   int     // 近期周期，如 60
	MaxRatio float64 // 最大上浮比例，如 0.15 表示价格在 60 日低点上浮 15% 以内
}

func NewNearLowFilter(period int, maxRatio float64) *NearLowFilter {
	return &NearLowFilter{Period: period, MaxRatio: maxRatio}
}

func (f *NearLowFilter) Filter(klines []*model.StockKline) []Result {
	n := len(klines)
	if n == 0 {
		return nil
	}

	results := make([]Result, n)
	for i := range n {
		start := 0
		if i >= f.Period {
			start = i - f.Period + 1
		}

		lowMin := klines[start].Low
		for j := start + 1; j <= i; j++ {
			if klines[j].Low < lowMin {
				lowMin = klines[j].Low
			}
		}

		maxAllowed := lowMin * (1 + f.MaxRatio)
		results[i] = Result{
			Date:  klines[i].Date,
			Valid: klines[i].Close <= maxAllowed,
		}
	}
	return results
}
