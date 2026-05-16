package strategy

import (
	"fmt"
	"testing"

	"trading/model"
)

func TestNewDailyB1BuyStrategy(t *testing.T) {
	st := NewDailyB1BuyStrategy()
	if st.Name() != "daily_b1_buy" {
		t.Fatalf("expected strategy name daily_b1_buy, got %s", st.Name())
	}
}

func TestNewWeeklyB1BuyStrategy(t *testing.T) {
	st := NewWeeklyB1BuyStrategy()
	if st.Name() != "weekly_b1_buy" {
		t.Fatalf("expected strategy name weekly_b1_buy, got %s", st.Name())
	}
}

func TestNewBottomSurgePullbackStrategy(t *testing.T) {
	st := NewBottomSurgePullbackStrategy()
	if st.Name() != "bottom_surge_pullback" {
		t.Fatalf("expected strategy name bottom_surge_pullback, got %s", st.Name())
	}
}

func TestBottomSurgePullbackStrategyScanAll(t *testing.T) {
	st := NewBottomSurgePullbackStrategy()

	// 构造满足条件的 K 线数据
	klines := make([]*model.StockKline, 68)
	for i := range 68 {
		price := 12.0
		if i >= 40 && i < 56 {
			price = 12.0 - float64(i-39)*0.125
		}
		klines[i] = &model.StockKline{
			Code:   "600312",
			Date:   fmt.Sprintf("2026-%02d-%02d", (i/30)+1, (i%30)+1),
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}

	// 倍量拉升（第 56 天，索引 56）
	klines[56] = &model.StockKline{
		Code: "600312", Date: "2026-02-26",
		Open: 10.0, High: 10.6, Low: 9.9, Close: 10.5, Volume: 350000,
	}
	// 峰值（第 57 天，索引 57）
	klines[57] = &model.StockKline{
		Code: "600312", Date: "2026-02-27",
		Open: 10.5, High: 10.7, Low: 10.4, Close: 10.6, Volume: 280000,
	}
	// 缩量回调（第 58-67 天，索引 58-67）
	for i := 58; i < 68; i++ {
		close := 10.25
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-02-%02d", i-56),
			Open: 10.3, High: 10.3, Low: 10.2, Close: close, Volume: 40000,
		}
	}

	signals := st.ScanAll(klines)
	if len(signals) == 0 {
		t.Fatal("expected at least one signal")
	}

	// 信号应该在回调的中后期出现
	lastSignal := signals[len(signals)-1]
	if lastSignal.Date != "2026-02-10" {
		t.Fatalf("expected last signal on 2026-02-10, got %s", lastSignal.Date)
	}
}

func TestBottomSurgePullbackStrategyNoMatch(t *testing.T) {
	st := NewBottomSurgePullbackStrategy()

	// 构造不满足条件的 K 线数据（没有倍量拉升）
	klines := make([]*model.StockKline, 30)
	for i := range 30 {
		price := 10.0 + float64(i)*0.01
		klines[i] = &model.StockKline{
			Code:   "600312",
			Date:   fmt.Sprintf("2026-01-%02d", i+1),
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}

	signals := st.ScanAll(klines)
	if len(signals) > 0 {
		t.Fatal("expected no signals for flat data")
	}
}

func TestBottomSurgePullbackStrategyEmpty(t *testing.T) {
	st := NewBottomSurgePullbackStrategy()
	signals := st.ScanAll(nil)
	if signals != nil {
		t.Fatal("expected nil for empty klines")
	}
}
