package financial

import (
	"testing"

	"trading/model"
)

func TestRevenueGrowthFilter(t *testing.T) {
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

	f := NewRevenueGrowthFilter().WithThreshold(0.1).WithQuarterCount(4)
	results := f.Filter(reports)

	if len(results) != len(reports) {
		t.Fatalf("expected %d results, got %d", len(reports), len(results))
	}

	for i := range 7 {
		if results[i].Valid {
			t.Fatalf("expected results[%d].Valid = false, got true", i)
		}
	}

	if !results[7].Valid {
		t.Fatal("expected results[7].Valid = true, got false")
	}
}

func TestRevenueGrowthFilterEmpty(t *testing.T) {
	f := NewRevenueGrowthFilter()
	results := f.Filter(nil)
	if results != nil {
		t.Fatalf("expected nil, got %v", results)
	}
}
