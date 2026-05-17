package filter

import (
	"testing"

	"trading/model"
)

func TestKDJRangeFilter(t *testing.T) {
	klines := make([]*model.StockKline, 20)
	for i := range 20 {
		price := 10.0 + float64(i)*0.1
		klines[i] = &model.StockKline{
			Date:   "2026-01-",
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}

	// 构造下跌让 KDJ 的 J 值走低
	for i := 5; i < 15; i++ {
		klines[i].Close = 8.0 - float64(i-5)*0.2
		klines[i].High = klines[i].Close + 0.1
		klines[i].Low = klines[i].Close - 0.1
	}

	f := NewKDJRangeFilter(-20, 20)
	results := f.Filter(klines)

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	found := false
	for _, r := range results {
		if r.Valid {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one Valid result in KDJ J-value range")
	}
}

func TestKDJRangeFilterEmpty(t *testing.T) {
	f := NewKDJRangeFilter(-20, 20)
	results := f.Filter(nil)
	if results != nil {
		t.Fatal("expected nil for empty klines")
	}
}

func TestKDJRangeFilterRejectsHighJ(t *testing.T) {
	klines := make([]*model.StockKline, 15)
	for i := range 15 {
		price := 10.0 + float64(i)*0.1
		klines[i] = &model.StockKline{
			Date:   "2026-01-",
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}

	// 持续上涨，J 值会偏高
	for i := 5; i < 15; i++ {
		klines[i].Close = 10.0 + float64(i-5)*0.5
		klines[i].High = klines[i].Close + 0.1
		klines[i].Low = klines[i].Close - 0.1
	}

	f := NewKDJRangeFilter(-20, 20)
	results := f.Filter(klines)

	for i, r := range results {
		if r.Valid {
			t.Fatalf("expected no Valid in high-J zone, got Valid at index %d", i)
		}
	}
}
