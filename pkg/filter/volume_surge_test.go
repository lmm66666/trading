package filter

import (
	"fmt"
	"testing"

	"trading/model"
)

func TestVolumeSurgeFilter(t *testing.T) {
	klines := make([]*model.StockKline, 70)
	for i := 0; i < 70; i++ {
		price := 10.0 + float64(i)*0.01
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
	// 放量上涨
	klines[30] = &model.StockKline{
		Code: "600312", Date: "2026-02-01",
		Open: 10.3, High: 10.7, Low: 10.2, Close: 10.6, Volume: 350000,
	}
	klines[31] = &model.StockKline{
		Code: "600312", Date: "2026-02-02",
		Open: 10.6, High: 11.0, Low: 10.5, Close: 10.9, Volume: 280000,
	}
	// 回调缩量
	for i := 32; i <= 46; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.12
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-02-%02d", i-29),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 60000,
		}
	}
	// 恢复
	for i := 47; i < 70; i++ {
		prevClose := klines[i-1].Close
		close := prevClose + 0.03
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-03-%02d", i-46),
			Open: prevClose, High: close + 0.05, Low: prevClose - 0.05, Close: close, Volume: 100000,
		}
	}

	cfg := DefaultVolumeSurgeConfig()
	cfg.MaxPullbackPct = 20.0
	cfg.MaxPullbackDays = 15
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// 放量上涨当天和峰值当天不应在回调窗口内（回调从 peak 之后开始）
	// 回调期间应 Valid=true
	foundValid := false
	for _, r := range results {
		if r.Valid {
			foundValid = true
			break
		}
	}
	if !foundValid {
		t.Fatal("expected at least one Valid result in pullback window")
	}
}

func TestVolumeSurgeFilterEmpty(t *testing.T) {
	f := NewVolumeSurgeFilter(DefaultVolumeSurgeConfig())
	results := f.Filter(nil)
	if results != nil {
		t.Fatal("expected nil for empty klines")
	}
}

func TestVolumeSurgeFilterWithPullbackVolumeCheck(t *testing.T) {
	klines := make([]*model.StockKline, 50)
	for i := 0; i < 50; i++ {
		price := 10.0 + float64(i)*0.01
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

	// 倍量拉升：量比 ~3.5，涨幅 6%
	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 10.25, High: 10.9, Low: 10.2, Close: 10.86, Volume: 350000,
	}
	// 峰值
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 11.0, Low: 10.8, Close: 11.0, Volume: 280000,
	}
	// 缩量回调：成交量大幅萎缩到 40000（< 拉升期均量 315000 的 50%）
	for i := 27; i <= 35; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 40000,
		}
	}

	cfg := DefaultVolumeSurgeConfig()
	cfg.MinVolumeRatio = 2.0
	cfg.MinRallyPct = 5.0
	cfg.MaxPullbackPct = 15.0
	cfg.MaxPullbackDays = 10
	cfg.MaxPullbackVolRatio = 0.5 // 缩量 50%
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// 缩量回调期间应该 Valid=true
	if !results[30].Valid {
		t.Fatal("expected Valid in pullback window with sufficient volume shrink")
	}
}

func TestVolumeSurgeFilterPullbackVolumeTooHigh(t *testing.T) {
	klines := make([]*model.StockKline, 50)
	for i := 0; i < 50; i++ {
		price := 10.0 + float64(i)*0.01
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

	// 倍量拉升
	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 10.25, High: 10.9, Low: 10.2, Close: 10.86, Volume: 350000,
	}
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 11.0, Low: 10.8, Close: 11.0, Volume: 280000,
	}
	// 回调期间成交量没有明显萎缩（200000，> 拉升期均量 315000 的 50%）
	for i := 27; i <= 35; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 200000,
		}
	}

	cfg := DefaultVolumeSurgeConfig()
	cfg.MinVolumeRatio = 2.0
	cfg.MinRallyPct = 5.0
	cfg.MaxPullbackPct = 15.0
	cfg.MaxPullbackDays = 10
	cfg.MaxPullbackVolRatio = 0.5
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	// 回调期间成交量太大，不满足缩量条件
	if results[30].Valid {
		t.Fatal("expected invalid when pullback volume is too high")
	}
}
