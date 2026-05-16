package filter

import (
	"testing"

	"trading/model"
)

func TestSupportHoldFilter(t *testing.T) {
	klines := make([]*model.StockKline, 25)
	for i := range 25 {
		klines[i] = &model.StockKline{
			Date:  "2026-01-",
			Close: 10.0 + float64(i)*0.1,
		}
	}

	f := NewSupportHoldFilter(5)
	results := f.Filter(klines)

	if len(results) != 25 {
		t.Fatalf("expected 25 results, got %d", len(results))
	}

	// 持续上涨，所有收盘价都在 MA5 上方
	for i := 5; i < 25; i++ {
		if !results[i].Valid {
			t.Fatalf("day %d should be valid in uptrend", i)
		}
	}
}

func TestSupportHoldFilterBreak(t *testing.T) {
	klines := make([]*model.StockKline, 15)
	for i := range 15 {
		price := 10.0 + float64(i)*0.1
		if i >= 10 {
			price = 5.0 // 大幅跌破
		}
		klines[i] = &model.StockKline{
			Date:  "2026-01-",
			Close: price,
		}
	}

	f := NewSupportHoldFilter(5)
	results := f.Filter(klines)

	// 大幅跌破后应该不满足
	if results[12].Valid {
		t.Fatal("expected invalid after major breakdown")
	}
}

func TestSupportHoldFilterSlightBreak(t *testing.T) {
	klines := make([]*model.StockKline, 15)
	for i := range 15 {
		price := 10.0 + float64(i)*0.1
		if i == 12 {
			price = 10.8 // 小幅跌破 MA5（约 11.0）
		}
		klines[i] = &model.StockKline{
			Date:  "2026-01-",
			Close: price,
		}
	}

	f := NewSupportHoldFilter(5)
	results := f.Filter(klines)

	// 小幅跌破（<2%）应该仍然满足
	if !results[12].Valid {
		t.Fatal("expected valid for slight break within 2%")
	}
}

func TestSupportHoldFilterEmpty(t *testing.T) {
	f := NewSupportHoldFilter(20)
	results := f.Filter(nil)
	if results != nil {
		t.Fatal("expected nil for empty klines")
	}
}
