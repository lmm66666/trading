package filter

import (
	"trading/model"
	"trading/pkg/indicator"
)

// MACrossFilter MA 短线在长线之上
type MACrossFilter struct {
	ShortPeriod int
	LongPeriod  int
}

func NewMACrossFilter(short, long int) *MACrossFilter {
	return &MACrossFilter{ShortPeriod: short, LongPeriod: long}
}

func (f *MACrossFilter) Filter(klines []*model.StockKline) []Result {
	n := len(klines)
	if n == 0 {
		return nil
	}

	prices := make([]float64, n)
	for i, k := range klines {
		prices[i] = k.Close
	}

	shortMA := indicator.ComputeMA(prices, f.ShortPeriod)
	longMA := indicator.ComputeMA(prices, f.LongPeriod)

	results := make([]Result, n)
	for i := range n {
		results[i] = Result{
			Date:  klines[i].Date,
			Valid: shortMA[i] > 0 && longMA[i] > 0 && shortMA[i] > longMA[i],
		}
	}
	return results
}
