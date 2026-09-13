package financialscreen

import "trading/model"

func NewProfitGrowth(threshold float64, quarterCount int) (ReportFilter, error) {
	return newGrowthFilter(threshold, quarterCount, func(report *model.FinancialReport) float64 {
		return report.NetProfit
	})
}
