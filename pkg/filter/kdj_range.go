package filter

import (
	"trading/model"
	"trading/pkg/indicator"
)

// KDJRangeFilter J 值在指定范围内的过滤器
type KDJRangeFilter struct {
	Min float64
	Max float64
}

func NewKDJRangeFilter(min, max float64) *KDJRangeFilter {
	return &KDJRangeFilter{Min: min, Max: max}
}

func (f *KDJRangeFilter) Filter(klines []*model.StockKline) []Result {
	if len(klines) == 0 {
		return nil
	}

	kdjResults := indicator.ComputeKDJ(klines)
	results := make([]Result, len(kdjResults))
	for i, r := range kdjResults {
		results[i] = Result{
			Date:  r.Date,
			Valid: r.J >= f.Min && r.J <= f.Max,
		}
	}
	return results
}
