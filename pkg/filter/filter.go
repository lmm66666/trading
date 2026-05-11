package filter

import (
	"trading/model"
)

type Result struct {
	Date  string
	Valid bool
}

type Filter interface {
	Filter(klines []*model.StockKline) []Result
}
