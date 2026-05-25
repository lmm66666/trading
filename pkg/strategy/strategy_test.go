package strategy

import (
	"fmt"
	"testing"

	"trading/model"
	"trading/pkg/filter"
)

func TestStrategyScanAll(t *testing.T) {
	klines := make([]*model.StockKline, 10)
	for i := 0; i < 10; i++ {
		klines[i] = &model.StockKline{
			Date:  fmt.Sprintf("2026-01-%02d", i+1),
			Close: 10.0 + float64(i)*0.1,
		}
	}

	s := NewStrategy("test").
		AddFilter(filter.NewMATrendUp(5, 1))

	signals := s.ScanAll(klines)
	if len(signals) == 0 {
		t.Fatal("expected signals from MA up trend")
	}

	// 验证所有信号日期都在 klines 中
	for _, sig := range signals {
		found := false
		for _, k := range klines {
			if k.Date == sig.Date {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("signal date %s not found in klines", sig.Date)
		}
	}
}

func TestStrategyScan(t *testing.T) {
	klines := make([]*model.StockKline, 10)
	for i := 0; i < 10; i++ {
		klines[i] = &model.StockKline{
			Date:  fmt.Sprintf("2026-01-%02d", i+1),
			Close: 10.0 + float64(i)*0.1,
		}
	}

	s := NewStrategy("test").
		AddFilter(filter.NewMATrendUp(5, 1))

	sig := s.Scan(klines)
	if sig == nil {
		t.Fatal("expected a signal")
	}

	if sig.Date == "" {
		t.Fatal("expected signal to have a date")
	}
}

func TestStrategyMultipleFilters(t *testing.T) {
	klines := make([]*model.StockKline, 20)
	for i := 0; i < 20; i++ {
		price := 10.0 + float64(i)*0.1
		klines[i] = &model.StockKline{
			Date:   fmt.Sprintf("2026-01-%02d", i+1),
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}

	// 两个条件：MA5 向上 + 价格 > 10.5
	s := NewStrategy("multi").
		AddFilter(filter.NewMATrendUp(5, 1)).
		AddFilter(&priceFilter{threshold: 10.5})

	signals := s.ScanAll(klines)

	// 价格 > 10.5 从第 6 天开始，MA5 向上从第 2 天开始
	// 交集应从第 6 天开始
	if len(signals) == 0 {
		t.Fatal("expected signals from multiple filters")
	}

	for _, sig := range signals {
		if sig.Date < "2026-01-06" {
			t.Fatalf("signal date %s should not appear before price > 10.5", sig.Date)
		}
	}
}

func TestStrategyNoFilter(t *testing.T) {
	klines := make([]*model.StockKline, 5)
	for i := 0; i < 5; i++ {
		klines[i] = &model.StockKline{Date: fmt.Sprintf("2026-01-%02d", i+1)}
	}

	s := NewStrategy("empty")
	if s.Scan(klines) != nil {
		t.Fatal("expected nil signal with no filters")
	}
	if s.ScanAll(klines) != nil {
		t.Fatal("expected nil signals with no filters")
	}
}

func TestStrategyEmptyKlines(t *testing.T) {
	s := NewStrategy("test").AddFilter(filter.NewMATrendUp(5, 1))
	if s.Scan(nil) != nil {
		t.Fatal("expected nil signal with empty klines")
	}
	if s.ScanAll(nil) != nil {
		t.Fatal("expected nil signals with empty klines")
	}
}

func TestScanLatest(t *testing.T) {
	klines := make([]*model.StockKline, 10)
	for i := 0; i < 10; i++ {
		price := 10.0 + float64(i)*0.1
		klines[i] = &model.StockKline{
			Date:  fmt.Sprintf("2026-01-%02d", i+1),
			Close: price,
		}
	}

	// MA5 向上 + 价格 > 10.5：最后一天满足
	s := NewStrategy("latest").
		AddFilter(filter.NewMATrendUp(5, 1)).
		AddFilter(&priceFilter{threshold: 10.5})

	sig := s.ScanLatest(klines)
	if sig == nil {
		t.Fatal("expected a signal on the latest day")
	}
	if sig.Date != "2026-01-10" {
		t.Fatalf("expected signal on 2026-01-10, got %s", sig.Date)
	}
}

func TestScanLatestNoMatch(t *testing.T) {
	klines := make([]*model.StockKline, 5)
	for i := 0; i < 5; i++ {
		klines[i] = &model.StockKline{
			Date:  fmt.Sprintf("2026-01-%02d", i+1),
			Close: 10.0,
		}
	}

	// 价格 > 20 不可能满足
	s := NewStrategy("no-match").
		AddFilter(&priceFilter{threshold: 20})

	if s.ScanLatest(klines) != nil {
		t.Fatal("expected nil when last day does not match")
	}
}

func TestScanLatestEmpty(t *testing.T) {
	s := NewStrategy("test").AddFilter(filter.NewMATrendUp(5, 1))
	if s.ScanLatest(nil) != nil {
		t.Fatal("expected nil for empty klines")
	}
	if s.ScanLatest([]*model.StockKline{}) != nil {
		t.Fatal("expected nil for zero-length klines")
	}
}

type priceFilter struct {
	threshold float64
}

func (f *priceFilter) Filter(klines []*model.StockKline) []filter.Result {
	results := make([]filter.Result, len(klines))
	for i, k := range klines {
		results[i] = filter.Result{
			Date:  k.Date,
			Valid: k.Close > f.threshold,
		}
	}
	return results
}
