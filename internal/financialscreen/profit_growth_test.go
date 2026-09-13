package financialscreen_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/financialscreen"
	"trading/model"
)

func TestProfitGrowthMatchesLegacyCases(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, NetProfit: 100},
		{ReportDate: "20230630", ReportType: 2, NetProfit: 250},
		{ReportDate: "20230930", ReportType: 3, NetProfit: 450},
		{ReportDate: "20231231", ReportType: 4, NetProfit: 700},
		{ReportDate: "20240331", ReportType: 1, NetProfit: 120},
		{ReportDate: "20240630", ReportType: 2, NetProfit: 300},
		{ReportDate: "20240930", ReportType: 3, NetProfit: 520},
		{ReportDate: "20241231", ReportType: 4, NetProfit: 800},
	}
	filter, err := financialscreen.NewProfitGrowth(0.1, 4)
	require.NoError(t, err)
	require.True(t, filter.Match(reports))

	reports[5].NetProfit = 250
	require.False(t, filter.Match(reports))
	for _, report := range reports {
		report.NetProfit = -report.NetProfit
	}
	require.False(t, filter.Match(reports))
}

func TestProfitGrowthPreservesMissingQuarterBehavior(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, NetProfit: 100},
		{ReportDate: "20231231", ReportType: 4, NetProfit: 250},
		{ReportDate: "20240331", ReportType: 1, NetProfit: 120},
		{ReportDate: "20241231", ReportType: 4, NetProfit: 300},
	}
	filter, err := financialscreen.NewProfitGrowth(0.1, 2)
	require.NoError(t, err)
	require.True(t, filter.Match(reports))
}
