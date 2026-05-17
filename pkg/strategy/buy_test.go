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

	// 构造满足条件的 K 线数据：
	// - 先横盘筑底（让 MA 趋平）
	// - 放量拉升突破（MA20 上穿 MA60）
	// - 回调让 J 回到 [-20,20]
	n := 100
	klines := make([]*model.StockKline, n)

	// 阶段1：长期横盘在 10.0 附近（index 0-69），让 MA20 ≈ MA60
	for i := 0; i < 70; i++ {
		price := 10.0 + 0.1*float64(i%10-5)*0.01
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

	// 阶段2：放量拉升（index 70-72），连续3天渐进放量
	klines[70] = &model.StockKline{
		Code: "600312", Date: "2026-03-11",
		Open: 10.0, High: 10.5, Low: 9.9, Close: 10.4, Volume: 220000,
	}
	klines[71] = &model.StockKline{
		Code: "600312", Date: "2026-03-12",
		Open: 10.4, High: 10.9, Low: 10.3, Close: 10.8, Volume: 250000,
	}
	klines[72] = &model.StockKline{
		Code: "600312", Date: "2026-03-13",
		Open: 10.8, High: 11.3, Low: 10.7, Close: 11.2, Volume: 230000,
	}

	// 峰值延续
	klines[73] = &model.StockKline{
		Code: "600312", Date: "2026-03-14",
		Open: 11.2, High: 11.3, Low: 11.0, Close: 11.1, Volume: 80000,
	}

	// 阶段3：回调（index 74-90），价格从 11.1 缓慢回落让 J 走低
	for i := 74; i < 91; i++ {
		close := 11.1 - float64(i-73)*0.08
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-03-%02d", i-72),
			Open: close + 0.02, High: close + 0.1, Low: close - 0.1, Close: close, Volume: 60000,
		}
	}

	// 后续恢复
	for i := 91; i < n; i++ {
		price := 9.8 + float64(i-91)*0.02
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-04-%02d", i-90),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}

	signals := st.ScanAll(klines)
	if len(signals) == 0 {
		t.Fatal("expected at least one signal")
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
