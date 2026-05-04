package strategy

import (
	"testing"

	"trading/model"
	"trading/pkg/filter/financial"
)

func TestFinancialStrategyScanAll(t *testing.T) {
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

	st := NewFinancialStrategy("test_financial").
		AddFilter(financial.NewProfitGrowthFilter().WithThreshold(0.1).WithQuarterCount(4))

	signals := st.ScanAll(reports)
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].ReportDate != "20241231" {
		t.Fatalf("expected signal date 20241231, got %s", signals[0].ReportDate)
	}
}

func TestFinancialStrategyScan(t *testing.T) {
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

	st := NewFinancialStrategy("test_financial").
		AddFilter(financial.NewProfitGrowthFilter().WithThreshold(0.1).WithQuarterCount(4))

	sig := st.Scan(reports)
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.ReportDate != "20241231" {
		t.Fatalf("expected signal date 20241231, got %s", sig.ReportDate)
	}
}

func TestFinancialStrategyNoMatch(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, NetProfit: 100},
		{ReportDate: "20240331", ReportType: 1, NetProfit: 90}, // yoy -10%
	}

	st := NewFinancialStrategy("test_financial").
		AddFilter(financial.NewProfitGrowthFilter().WithThreshold(0.1).WithQuarterCount(4))

	sig := st.Scan(reports)
	if sig != nil {
		t.Fatalf("expected nil signal, got %v", sig)
	}
}

func TestFinancialStrategyEmptyFilters(t *testing.T) {
	st := NewFinancialStrategy("empty")
	sig := st.Scan([]*model.FinancialReport{
		{ReportDate: "20240131", NetProfit: 100},
	})
	if sig != nil {
		t.Fatal("expected nil with no filters")
	}
}

func TestFinancialStrategyEmptyReports(t *testing.T) {
	st := NewFinancialStrategy("empty").
		AddFilter(financial.NewProfitGrowthFilter())
	sig := st.Scan(nil)
	if sig != nil {
		t.Fatal("expected nil with empty reports")
	}
}
