package filter

import (
	"trading/model"
	"trading/pkg/indicator"
)

// KDJRangeFilter KDJ 在指定范围内的过滤器
type KDJRangeFilter struct {
	MinK float64 // K 值下限
	MaxK float64 // K 值上限
}

func NewKDJRangeFilter(minK, maxK float64) *KDJRangeFilter {
	return &KDJRangeFilter{MinK: minK, MaxK: maxK}
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
			Valid: r.K >= f.MinK && r.K <= f.MaxK,
		}
	}
	return results
}
