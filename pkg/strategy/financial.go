package strategy

import (
	"trading/model"
	"trading/pkg/filter/financial"
)

// FinancialStrategy 财报策略，组合多个财报 filter
type FinancialStrategy struct {
	name    string
	filters []financial.FinancialFilter
}

// NewFinancialStrategy 创建财报策略
func NewFinancialStrategy(name string) *FinancialStrategy {
	return &FinancialStrategy{
		name:    name,
		filters: []financial.FinancialFilter{},
	}
}

// Name 返回策略名称
func (s *FinancialStrategy) Name() string {
	return s.name
}

// AddFilter 添加 filter，支持链式调用
func (s *FinancialStrategy) AddFilter(f financial.FinancialFilter) *FinancialStrategy {
	s.filters = append(s.filters, f)
	return s
}

// ScanAll 返回所有满足全部 filter 条件的报告期
func (s *FinancialStrategy) ScanAll(reports []*model.FinancialReport) []financial.Signal {
	if len(s.filters) == 0 || len(reports) == 0 {
		return nil
	}

	n := len(reports)
	allResults := make([][]financial.Result, len(s.filters))
	for i, f := range s.filters {
		allResults[i] = f.Filter(reports)
	}

	var signals []financial.Signal
	for day := range n {
		allValid := true
		for i := range allResults {
			if day >= len(allResults[i]) || !allResults[i][day].Valid {
				allValid = false
				break
			}
		}
		if allValid {
			// 需要获取对应的 report_date
			reportDate := allResults[0][day].ReportDate
			signals = append(signals, financial.Signal{ReportDate: reportDate})
		}
	}
	return signals
}

// Scan 返回最近一个满足全部 filter 条件的报告期
func (s *FinancialStrategy) Scan(reports []*model.FinancialReport) *financial.Signal {
	if len(s.filters) == 0 || len(reports) == 0 {
		return nil
	}

	n := len(reports)
	allResults := make([][]financial.Result, len(s.filters))
	for i, f := range s.filters {
		allResults[i] = f.Filter(reports)
	}

	for day := n - 1; day >= 0; day-- {
		allValid := true
		for i := range allResults {
			if day >= len(allResults[i]) || !allResults[i][day].Valid {
				allValid = false
				break
			}
		}
		if allValid {
			reportDate := allResults[0][day].ReportDate
			return &financial.Signal{ReportDate: reportDate}
		}
	}
	return nil
}
