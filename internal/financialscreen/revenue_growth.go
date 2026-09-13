package financialscreen

import "trading/model"

func NewRevenueGrowth(threshold float64, quarterCount int) (ReportFilter, error) {
	return newGrowthFilter(threshold, quarterCount, func(report *model.FinancialReport) float64 {
		return report.TotalRevenue
	})
}
