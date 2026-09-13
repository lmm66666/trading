package financialscreen_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/financialscreen"
	"trading/model"
)

func TestRevenueGrowthMatchesLegacyCases(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, TotalRevenue: 1000},
		{ReportDate: "20230630", ReportType: 2, TotalRevenue: 2500},
		{ReportDate: "20230930", ReportType: 3, TotalRevenue: 4500},
		{ReportDate: "20231231", ReportType: 4, TotalRevenue: 7000},
		{ReportDate: "20240331", ReportType: 1, TotalRevenue: 1200},
		{ReportDate: "20240630", ReportType: 2, TotalRevenue: 3000},
		{ReportDate: "20240930", ReportType: 3, TotalRevenue: 5200},
		{ReportDate: "20241231", ReportType: 4, TotalRevenue: 8000},
	}
	filter, err := financialscreen.NewRevenueGrowth(0.1, 4)
	require.NoError(t, err)
	require.True(t, filter.Match(reports))
	require.False(t, filter.Match(nil))
}
