package financial

import "trading/model"

// ProfitGrowthFilter 净利润同比增长 filter
type ProfitGrowthFilter struct {
	Threshold    float64 // 最低同比增长率，如 0.1 表示 10%
	QuarterCount int     // 需要连续满足的季度数，默认 4（一年）
}

// NewProfitGrowthFilter 创建净利润同比增长 filter，默认连续 4 个季度增长超过 10%
func NewProfitGrowthFilter() *ProfitGrowthFilter {
	return &ProfitGrowthFilter{Threshold: 0.1, QuarterCount: 4}
}

// WithThreshold 设置最低同比增长率阈值，支持链式调用
func (f *ProfitGrowthFilter) WithThreshold(t float64) *ProfitGrowthFilter {
	f.Threshold = t
	return f
}

// WithQuarterCount 设置连续季度数，支持链式调用
func (f *ProfitGrowthFilter) WithQuarterCount(n int) *ProfitGrowthFilter {
	f.QuarterCount = n
	return f
}

// Filter 对每个报告期，检查从该期往前连续 QuarterCount 个季度是否均满足净利润同比增长 >= Threshold
func (f *ProfitGrowthFilter) Filter(reports []*model.FinancialReport) []Result {
	return computeGrowthFilter(reports, f.Threshold, f.QuarterCount, func(r *model.FinancialReport) float64 {
		return r.NetProfit
	})
}
