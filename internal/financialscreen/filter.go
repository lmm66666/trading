package financialscreen

import (
	"errors"
	"math"
	"sort"
	"strconv"

	"trading/model"
)

var ErrInvalidFilter = errors.New("invalid financial screen filter")

type ReportFilter interface {
	Match(reports []*model.FinancialReport) bool
}

type valueExtractor func(*model.FinancialReport) float64

type growthFilter struct {
	threshold    float64
	quarterCount int
	extract      valueExtractor
}

func newGrowthFilter(threshold float64, quarterCount int, extract valueExtractor) (ReportFilter, error) {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || quarterCount < 1 || extract == nil {
		return nil, ErrInvalidFilter
	}
	return growthFilter{threshold: threshold, quarterCount: quarterCount, extract: extract}, nil
}

func (f growthFilter) Match(reports []*model.FinancialReport) bool {
	if len(reports) < f.quarterCount {
		return false
	}
	sorted := append([]*model.FinancialReport(nil), reports...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ReportDate < sorted[j].ReportDate })
	type key struct{ year, reportType int }
	cumulative := make(map[key]float64, len(sorted))
	for _, report := range sorted {
		if year := reportYear(report.ReportDate); year > 0 {
			cumulative[key{year, report.ReportType}] = f.extract(report)
		}
	}
	quarterly := make(map[key]float64, len(cumulative))
	for _, report := range sorted {
		year := reportYear(report.ReportDate)
		if year == 0 {
			continue
		}
		current := key{year, report.ReportType}
		quarterly[current] = cumulative[current]
		if report.ReportType != 1 {
			if previous, ok := cumulative[key{year, report.ReportType - 1}]; ok {
				quarterly[current] -= previous
			}
		}
	}
	start := len(sorted) - f.quarterCount
	for _, report := range sorted[start:] {
		year := reportYear(report.ReportDate)
		current := quarterly[key{year, report.ReportType}]
		previous, ok := quarterly[key{year - 1, report.ReportType}]
		if !ok || previous == 0 || current <= 0 || (current-previous)/previous < f.threshold {
			return false
		}
	}
	return true
}

func reportYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(date[:4])
	return year
}
