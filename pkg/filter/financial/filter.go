package financial

import (
	"sort"
	"strconv"

	"trading/model"
)

type Result struct {
	ReportDate string
	Valid      bool
}

type Signal struct {
	ReportDate string
}

type IFinancialFilter interface {
	Filter(reports []*model.FinancialReport) []Result
}

// valueExtractor 从财报记录中提取需要比较的数值
// 使用函数而非接口避免引入不必要的复杂度
type valueExtractor func(r *model.FinancialReport) float64

// computeGrowthFilter 通用同比增长 filter 实现
// 注意：财报季度数据为累加值（半年报含Q1+Q2，三季报含Q1+Q2+Q3，年报为全年），
// 计算前会先转换为单季度值，再比较同比
func computeGrowthFilter(reports []*model.FinancialReport, threshold float64, quarterCount int, extract valueExtractor) []Result {
	if len(reports) == 0 {
		return nil
	}

	sorted := make([]*model.FinancialReport, len(reports))
	copy(sorted, reports)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ReportDate < sorted[j].ReportDate
	})

	type key struct{ year, reportType int }

	// 1. 建立累计值索引
	cumulative := make(map[key]float64)
	for _, r := range sorted {
		year := parseYear(r.ReportDate)
		if year > 0 {
			cumulative[key{year, r.ReportType}] = extract(r)
		}
	}

	// 2. 累计值转换为单季度值
	quarterly := make(map[key]float64)
	for _, r := range sorted {
		year := parseYear(r.ReportDate)
		if year == 0 {
			continue
		}
		k := key{year, r.ReportType}
		if r.ReportType == 1 {
			quarterly[k] = cumulative[k]
		} else {
			prevK := key{year, r.ReportType - 1}
			if prevVal, ok := cumulative[prevK]; ok {
				quarterly[k] = cumulative[k] - prevVal
			} else {
				quarterly[k] = cumulative[k]
			}
		}
	}

	// 3. 计算每个季度自身是否满足同比增长
	results := make([]Result, len(sorted))
	for i, r := range sorted {
		year := parseYear(r.ReportDate)
		currK := key{year, r.ReportType}
		prevK := key{year: year - 1, reportType: r.ReportType}

		currVal := quarterly[currK]
		prevVal, ok := quarterly[prevK]

		var valid bool
		if ok && prevVal != 0 && currVal > 0 {
			growth := (currVal - prevVal) / prevVal
			if growth >= threshold {
				valid = true
			}
		}

		results[i] = Result{
			ReportDate: r.ReportDate,
			Valid:      valid,
		}
	}

	// 4. 只检查最近 quarterCount 个报告期是否都满足
	// 如果都满足，仅最新一期标记 Valid=true（其余重置为 false）
	if len(results) >= quarterCount {
		allValid := true
		for i := len(results) - quarterCount; i < len(results); i++ {
			if !results[i].Valid {
				allValid = false
				break
			}
		}
		for i := range results {
			results[i].Valid = false
		}
		if allValid {
			results[len(results)-1].Valid = true
		}
	} else {
		// 数据不足 quarterCount 个，全部不满足
		for i := range results {
			results[i].Valid = false
		}
	}

	return results
}

func parseYear(reportDate string) int {
	if len(reportDate) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(reportDate[:4])
	return year
}
