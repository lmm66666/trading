package filter

import (
	"testing"

	"trading/model"
)

func TestDailyMA20CrossFilter(t *testing.T) {
	klines := []*model.StockKline{
		{Date: "2026-01-01", Close: 12.0, AuxMA20: 10.0},
		{Date: "2026-01-02", Close: 9.0, AuxMA20: 10.0},
		{Date: "2026-01-03", Close: 10.5, AuxMA20: 10.0},
	}

	f := NewDailyMA20CrossFilter()
	results := f.Filter(klines)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Valid {
		t.Fatal("expected day 0 valid: close 12.0 >= ma20 10.0")
	}
	if results[1].Valid {
		t.Fatal("expected day 1 invalid: close 9.0 < ma20 10.0")
	}
	if !results[2].Valid {
		t.Fatal("expected day 2 valid: close 10.5 >= ma20 10.0")
	}
}

func TestDailyMA20CrossFilterZeroMA(t *testing.T) {
	klines := []*model.StockKline{
		{Date: "2026-01-01", Close: 12.0, AuxMA20: 0},
	}

	f := NewDailyMA20CrossFilter()
	results := f.Filter(klines)

	if results[0].Valid {
		t.Fatal("expected invalid when AuxMA20 is 0")
	}
}

func TestDailyMA20CrossFilterEmpty(t *testing.T) {
	f := NewDailyMA20CrossFilter()
	results := f.Filter(nil)
	if results != nil {
		t.Fatal("expected nil for empty klines")
	}
}
