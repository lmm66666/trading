package filter

import (
	"trading/model"
)

// DailyMA20CrossFilter 收盘价站稳日 20 日均线的跨周期过滤器。
// 依赖 klines[i].AuxMA20 已预先填充日 20 均线值。
type DailyMA20CrossFilter struct{}

func NewDailyMA20CrossFilter() *DailyMA20CrossFilter {
	return &DailyMA20CrossFilter{}
}

func (f *DailyMA20CrossFilter) Filter(klines []*model.StockKline) []Result {
	n := len(klines)
	if n == 0 {
		return nil
	}

	results := make([]Result, n)
	for i := range n {
		results[i] = Result{
			Date:  klines[i].Date,
			Valid: klines[i].AuxMA20 > 0 && klines[i].Close >= klines[i].AuxMA20,
		}
	}
	return results
}
