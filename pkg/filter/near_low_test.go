package filter

import (
	"testing"

	"trading/model"
)

func TestNearLowFilter(t *testing.T) {
	klines := make([]*model.StockKline, 20)
	for i := range 20 {
		klines[i] = &model.StockKline{
			Date:   "2026-01-",
			Low:    8.0 + float64(i)*0.1,
			Close:  10.0 + float64(i)*0.1,
			Volume: 100000,
		}
	}

	f := NewNearLowFilter(10, 0.15)
	results := f.Filter(klines)

	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}

	// 前几天数据不足，低点从第0天开始，Close=10.0，Low=8.0
	// maxAllowed = 8.0 * 1.15 = 9.2，Close=10.0 > 9.2，应该不满足
	if results[0].Valid {
		t.Fatal("day 0 should not be valid, close is far from 60-day low")
	}
}

func TestNearLowFilterAtLow(t *testing.T) {
	klines := make([]*model.StockKline, 15)
	for i := range 15 {
		klines[i] = &model.StockKline{
			Date:  "2026-01-",
			Low:   10.0,
			Close: 10.5,
		}
	}

	f := NewNearLowFilter(10, 0.10)
	results := f.Filter(klines)

	// Close=10.5, lowMin=10.0, maxAllowed=10.0*1.10=11.0
	// 10.5 <= 11.0，应该满足
	if !results[10].Valid {
		t.Fatal("expected valid when close is within 10% of recent low")
	}
}

func TestNearLowFilterEmpty(t *testing.T) {
	f := NewNearLowFilter(10, 0.15)
	results := f.Filter(nil)
	if results != nil {
		t.Fatal("expected nil for empty klines")
	}
}
