package scorer

import (
	"testing"

	"trading/model"
)

func buildFinancialReports() []*model.FinancialReport {
	return []*model.FinancialReport{
		{Code: "600000", ReportDate: "20240331", ReportType: 1, NetProfit: 100, TotalRevenue: 500, NetProfitCut: 92, GrossMargin: 35.0, ROE: 3.5, AssetLiabilityRatio: 40.0, CurrentRatio: 2.2, QuickRatio: 1.2, TotalAssetTurnover: 0.3, EPS: 0.5, OperatingCashFlow: 80},
		{Code: "600000", ReportDate: "20240630", ReportType: 2, NetProfit: 210, TotalRevenue: 1050, NetProfitCut: 195, GrossMargin: 36.0, ROE: 7.0, AssetLiabilityRatio: 42.0, CurrentRatio: 2.1, QuickRatio: 1.1, TotalAssetTurnover: 0.6, EPS: 1.0, OperatingCashFlow: 170},
		{Code: "600000", ReportDate: "20240930", ReportType: 3, NetProfit: 320, TotalRevenue: 1600, NetProfitCut: 300, GrossMargin: 37.0, ROE: 10.5, AssetLiabilityRatio: 41.0, CurrentRatio: 2.3, QuickRatio: 1.15, TotalAssetTurnover: 0.9, EPS: 1.5, OperatingCashFlow: 260},
		{Code: "600000", ReportDate: "20241231", ReportType: 4, NetProfit: 450, TotalRevenue: 2200, NetProfitCut: 420, GrossMargin: 38.0, ROE: 14.0, AssetLiabilityRatio: 39.0, CurrentRatio: 2.4, QuickRatio: 1.3, TotalAssetTurnover: 1.2, EPS: 2.0, OperatingCashFlow: 380},
		{Code: "600000", ReportDate: "20250331", ReportType: 1, NetProfit: 120, TotalRevenue: 600, NetProfitCut: 110, GrossMargin: 38.5, ROE: 4.0, AssetLiabilityRatio: 38.0, CurrentRatio: 2.5, QuickRatio: 1.4, TotalAssetTurnover: 0.35, EPS: 0.6, OperatingCashFlow: 100},
		{Code: "600000", ReportDate: "20250630", ReportType: 2, NetProfit: 250, TotalRevenue: 1250, NetProfitCut: 230, GrossMargin: 39.0, ROE: 8.0, AssetLiabilityRatio: 37.0, CurrentRatio: 2.3, QuickRatio: 1.3, TotalAssetTurnover: 0.7, EPS: 1.2, OperatingCashFlow: 200},
		{Code: "600000", ReportDate: "20250930", ReportType: 3, NetProfit: 380, TotalRevenue: 1900, NetProfitCut: 355, GrossMargin: 39.5, ROE: 12.0, AssetLiabilityRatio: 36.0, CurrentRatio: 2.6, QuickRatio: 1.5, TotalAssetTurnover: 1.0, EPS: 1.8, OperatingCashFlow: 310},
		{Code: "600000", ReportDate: "20251231", ReportType: 4, NetProfit: 520, TotalRevenue: 2600, NetProfitCut: 490, GrossMargin: 40.0, ROE: 16.0, AssetLiabilityRatio: 35.0, CurrentRatio: 2.7, QuickRatio: 1.6, TotalAssetTurnover: 1.3, EPS: 2.4, OperatingCashFlow: 430},
	}
}

func TestScoreLongFinancial_InsufficientData(t *testing.T) {
	reports := []*model.FinancialReport{
		{Code: "600000", ReportDate: "20251231", ReportType: 4},
		{Code: "600000", ReportDate: "20250930", ReportType: 3},
	}
	result := ScoreLongFinancial(reports)
	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items should be empty for insufficient data, got %d items", len(result.Items))
	}
}

func TestScoreLongFinancial_IdealCase(t *testing.T) {
	reports := buildFinancialReports()
	result := ScoreLongFinancial(reports)

	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	if len(result.Items) == 0 {
		t.Fatal("Items should not be empty for valid financial reports")
	}

	// 验证 MaxScore 合计 = 100
	maxSum := 0
	for _, item := range result.Items {
		maxSum += item.MaxScore
		if item.Score < 0 || item.Score > item.MaxScore {
			t.Errorf("item %q: Score=%d out of range [0, %d]", item.Name, item.Score, item.MaxScore)
		}
	}
	if maxSum != 100 {
		t.Errorf("MaxScore sum = %d, want 100", maxSum)
	}

	// 理想情况下总分应较高（盈利增长、盈利质量、财务安全都好）
	if result.Total < 50 {
		t.Errorf("Total = %d, too low for ideal financial case", result.Total)
	}
}

func TestScoreLongFinancial_PoorCompany(t *testing.T) {
	// 亏损公司：净利润下滑、负债高、现金流差
	reports := []*model.FinancialReport{
		{Code: "000001", ReportDate: "20240331", ReportType: 1, NetProfit: 50, TotalRevenue: 300, NetProfitCut: 60, GrossMargin: 15.0, ROE: 1.0, AssetLiabilityRatio: 75.0, CurrentRatio: 0.8, QuickRatio: 0.3, TotalAssetTurnover: 0.2, EPS: 0.1, OperatingCashFlow: -10},
		{Code: "000001", ReportDate: "20240630", ReportType: 2, NetProfit: 40, TotalRevenue: 280, NetProfitCut: 55, GrossMargin: 14.0, ROE: 0.8, AssetLiabilityRatio: 76.0, CurrentRatio: 0.7, QuickRatio: 0.25, TotalAssetTurnover: 0.15, EPS: 0.08, OperatingCashFlow: -20},
		{Code: "000001", ReportDate: "20240930", ReportType: 3, NetProfit: 30, TotalRevenue: 260, NetProfitCut: 45, GrossMargin: 13.0, ROE: 0.5, AssetLiabilityRatio: 78.0, CurrentRatio: 0.6, QuickRatio: 0.2, TotalAssetTurnover: 0.1, EPS: 0.05, OperatingCashFlow: -30},
		{Code: "000001", ReportDate: "20241231", ReportType: 4, NetProfit: 20, TotalRevenue: 250, NetProfitCut: 35, GrossMargin: 12.0, ROE: 0.3, AssetLiabilityRatio: 80.0, CurrentRatio: 0.5, QuickRatio: 0.15, TotalAssetTurnover: 0.08, EPS: 0.02, OperatingCashFlow: -40},
	}
	result := ScoreLongFinancial(reports)

	if result.Total >= 60 {
		t.Errorf("Total = %d, should be low for poor company", result.Total)
	}
}

func TestProfitYoYScore(t *testing.T) {
	tests := []struct {
		name  string
		v     float64
		want  int
	}{
		{"high_growth", 0.35, 20},
		{"moderate_growth", 0.15, 15},
		{"low_growth", 0.05, 8},
		{"flat", 0.0, 8},
		{"slight_decline", -0.05, 4},
		{"moderate_decline", -0.2, 2},
		{"severe_decline", -0.4, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profitYoYScore(tt.v)
			if got != tt.want {
				t.Errorf("profitYoYScore(%v) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestRevYoYScore(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want int
	}{
		{"high", 0.25, 10},
		{"moderate", 0.10, 7},
		{"low", 0.02, 4},
		{"negative", -0.05, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := revYoYScore(tt.v)
			if got != tt.want {
				t.Errorf("revYoYScore(%v) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestDebtScore(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want int
	}{
		{"low", 0.3, 10},
		{"medium", 0.6, 5},
		{"high", 0.8, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := debtScore(tt.v)
			if got != tt.want {
				t.Errorf("debtScore(%v) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestYoy(t *testing.T) {
	reports := []*model.FinancialReport{
		{Code: "600000", ReportDate: "20241231", ReportType: 4, NetProfit: 100},
		{Code: "600000", ReportDate: "20251231", ReportType: 4, NetProfit: 130},
	}
	got := yoy(reports, func(r *model.FinancialReport) float64 { return r.NetProfit })
	want := 0.3
	if got != want {
		t.Errorf("yoy() = %v, want %v", got, want)
	}
}

func TestYoy_NoPreviousYear(t *testing.T) {
	reports := []*model.FinancialReport{
		{Code: "600000", ReportDate: "20251231", ReportType: 4, NetProfit: 130},
	}
	got := yoy(reports, func(r *model.FinancialReport) float64 { return r.NetProfit })
	if got != -999 {
		t.Errorf("yoy() without previous year = %v, want -999", got)
	}
}

func TestYoy_ZeroBase(t *testing.T) {
	reports := []*model.FinancialReport{
		{Code: "600000", ReportDate: "20241231", ReportType: 4, NetProfit: 0},
		{Code: "600000", ReportDate: "20251231", ReportType: 4, NetProfit: 100},
	}
	got := yoy(reports, func(r *model.FinancialReport) float64 { return r.NetProfit })
	if got != -999 {
		t.Errorf("yoy() with zero base = %v, want -999", got)
	}
}

func TestTrend(t *testing.T) {
	reports := []*model.FinancialReport{
		{Code: "600000", ReportDate: "20250331", ReportType: 1, GrossMargin: 35.0},
		{Code: "600000", ReportDate: "20250630", ReportType: 2, GrossMargin: 37.0},
		{Code: "600000", ReportDate: "20250930", ReportType: 3, GrossMargin: 39.0},
		{Code: "600000", ReportDate: "20251231", ReportType: 4, GrossMargin: 40.0},
	}
	got := trend(reports, func(r *model.FinancialReport) float64 { return r.GrossMargin / 100 })
	// (40/100) - (35/100) = 0.05
	if got < 0.049 || got > 0.051 {
		t.Errorf("trend() = %v, want ~0.05", got)
	}
}

func TestEpsTrendScore(t *testing.T) {
	tests := []struct {
		name    string
		epsVals []float64
		want    int
	}{
		{"increasing", []float64{0.5, 0.8, 1.0, 1.2}, 7},
		{"decreasing", []float64{1.5, 1.2, 0.8, 0.5}, 0},
		{"mixed", []float64{0.5, 1.0, 0.8, 1.2}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reports := make([]*model.FinancialReport, len(tt.epsVals))
			for i, v := range tt.epsVals {
				reports[i] = &model.FinancialReport{EPS: v}
			}
			got := epsTrendScore(reports)
			if got != tt.want {
				t.Errorf("epsTrendScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCutRatioScore(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want int
	}{
		{"high", 0.95, 5},
		{"medium", 0.8, 3},
		{"low", 0.5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cutRatioScore(tt.v)
			if got != tt.want {
				t.Errorf("cutRatioScore(%v) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}
