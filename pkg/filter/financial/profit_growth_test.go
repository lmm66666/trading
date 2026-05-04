package financial

import (
	"testing"

	"trading/model"
)

// TestProfitGrowthFilter 验证累计财报数据先转为单季度再计算同比
// 数据说明：
//   2023Q1=100, Q2=150(cum=250), Q3=200(cum=450), Q4=250(cum=700)
//   2024Q1=120(yoy+20%), Q2=180(cum=300, yoy+20%), Q3=220(cum=520, yoy+10%), Q4=280(cum=800, yoy+12%)
func TestProfitGrowthFilter(t *testing.T) {
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

	f := NewProfitGrowthFilter().WithThreshold(0.1).WithQuarterCount(4)
	results := f.Filter(reports)

	if len(results) != len(reports) {
		t.Fatalf("expected %d results, got %d", len(reports), len(results))
	}

	// 前 7 个季度连续不足 4 个，应该为 false
	for i := range 7 {
		if results[i].Valid {
			t.Fatalf("expected results[%d].Valid = false, got true", i)
		}
	}

	// 2024Q4 开始连续 4 个单季度满足条件
	if !results[7].Valid {
		t.Fatal("expected results[7].Valid = true, got false")
	}
}

// TestProfitGrowthFilterNotEnoughConsecutive 中间断档导致无法连续 4 个季度满足
func TestProfitGrowthFilterNotEnoughConsecutive(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, NetProfit: 100},
		{ReportDate: "20230630", ReportType: 2, NetProfit: 250},
		{ReportDate: "20230930", ReportType: 3, NetProfit: 450},
		{ReportDate: "20231231", ReportType: 4, NetProfit: 700},
		{ReportDate: "20240331", ReportType: 1, NetProfit: 120}, // Q1 yoy +20%
		{ReportDate: "20240630", ReportType: 2, NetProfit: 250}, // Q2=130, yoy (130-150)/150=-13.3% (不满足)
		{ReportDate: "20240930", ReportType: 3, NetProfit: 500}, // Q3=250, yoy (250-200)/200=+25%
		{ReportDate: "20241231", ReportType: 4, NetProfit: 780}, // Q4=280, yoy (280-250)/250=+12%
	}

	f := NewProfitGrowthFilter().WithThreshold(0.1).WithQuarterCount(4)
	results := f.Filter(reports)

	// 2024Q2 不满足，连续计数断裂，没有任何一期能连续 4 个季度满足
	for i, r := range results {
		if r.Valid {
			t.Fatalf("expected results[%d].Valid = false, got true", i)
		}
	}
}

// TestProfitGrowthFilterNegativeProfit 净利润为负即使增长达标也不通过
// 亏损从 -20% 收窄到 -10% 不可取，净利润必须为正
func TestProfitGrowthFilterNegativeProfit(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, NetProfit: -100},
		{ReportDate: "20230630", ReportType: 2, NetProfit: -250},
		{ReportDate: "20230930", ReportType: 3, NetProfit: -450},
		{ReportDate: "20231231", ReportType: 4, NetProfit: -700},
		{ReportDate: "20240331", ReportType: 1, NetProfit: -80},  // Q1: -80 vs -100, yoy +20%
		{ReportDate: "20240630", ReportType: 2, NetProfit: -200}, // Q2: -120 vs -150, yoy +20%
		{ReportDate: "20240930", ReportType: 3, NetProfit: -360}, // Q3: -160 vs -200, yoy +20%
		{ReportDate: "20241231", ReportType: 4, NetProfit: -560}, // Q4: -200 vs -250, yoy +20%
	}

	f := NewProfitGrowthFilter().WithThreshold(0.1).WithQuarterCount(4)
	results := f.Filter(reports)

	for i, r := range results {
		if r.Valid {
			t.Fatalf("expected results[%d].Valid = false (negative profit), got true", i)
		}
	}
}

func TestProfitGrowthFilterEmpty(t *testing.T) {
	f := NewProfitGrowthFilter()
	results := f.Filter(nil)
	if results != nil {
		t.Fatalf("expected nil, got %v", results)
	}
}
