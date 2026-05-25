package strategy

import (
	"trading/model"
	"trading/pkg/filter"
)

type Signal struct {
	Date string
}

type Strategy struct {
	name    string
	filters []filter.Filter
}

func NewStrategy(name string) *Strategy {
	return &Strategy{
		name:    name,
		filters: []filter.Filter{},
	}
}

func (s *Strategy) Name() string {
	return s.name
}

func (s *Strategy) AddFilter(f filter.Filter) *Strategy {
	s.filters = append(s.filters, f)
	return s
}

// ScanAll 返回所有满足全部 filter 条件的信号
func (s *Strategy) ScanAll(klines []*model.StockKline) []Signal {
	if len(s.filters) == 0 || len(klines) == 0 {
		return nil
	}

	n := len(klines)
	allResults := make([][]filter.Result, len(s.filters))
	for i, f := range s.filters {
		allResults[i] = f.Filter(klines)
	}

	var signals []Signal
	for day := range n {
		allValid := true
		for i := range allResults {
			if day >= len(allResults[i]) || !allResults[i][day].Valid {
				allValid = false
				break
			}
		}
		if allValid {
			signals = append(signals, Signal{Date: klines[day].Date})
		}
	}
	return signals
}

// Scan 返回第一个满足全部 filter 条件的信号
func (s *Strategy) Scan(klines []*model.StockKline) *Signal {
	if len(s.filters) == 0 || len(klines) == 0 {
		return nil
	}

	n := len(klines)
	allResults := make([][]filter.Result, len(s.filters))
	for i, f := range s.filters {
		allResults[i] = f.Filter(klines)
	}

	for day := range n {
		allValid := true
		for i := range allResults {
			if day >= len(allResults[i]) || !allResults[i][day].Valid {
				allValid = false
				break
			}
		}
		if allValid {
			return &Signal{Date: klines[day].Date}
		}
	}
	return nil
}

// ScanLatest 只检查最后一天是否满足全部 filter 条件
func (s *Strategy) ScanLatest(klines []*model.StockKline) *Signal {
	if len(s.filters) == 0 || len(klines) == 0 {
		return nil
	}

	lastIdx := len(klines) - 1
	allResults := make([][]filter.Result, len(s.filters))
	for i, f := range s.filters {
		allResults[i] = f.Filter(klines)
	}

	for i := range allResults {
		if lastIdx >= len(allResults[i]) || !allResults[i][lastIdx].Valid {
			return nil
		}
	}
	return &Signal{Date: klines[lastIdx].Date}
}
