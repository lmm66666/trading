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

	if results[30].Valid {
		t.Fatal("expected invalid when pullback volume is too high")
	}
}

func TestVolumeSurgeFilterWithNearLowAtSurgeDay(t *testing.T) {
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

	// 底部区域：index 20-24 价格在 8.5~9.0
	for i := 20; i <= 24; i++ {
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-19),
			Open: 8.6, High: 9.0, Low: 8.4, Close: 8.7, Volume: 80000,
		}
	}

	// 放量日 Open 在底部范围内（8.5 < 8.4*1.15=9.66），Close 已拉升
	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 8.5, High: 10.9, Low: 8.4, Close: 10.86, Volume: 350000,
	}
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 11.0, Low: 10.8, Close: 11.0, Volume: 280000,
	}
	// 回调
	for i := 27; i <= 35; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 40000,
		}
	}

	// NearLow: 放量日 Open=8.5，60日低点约8.4，上限=8.4*1.15=9.66 → 8.5<9.66 通过
	cfg := VolumeSurgeConfig{
		VolumeMAPeriod:   20,
		MinVolumeRatio:   2.0,
		MinRallyPct:      5.0,
		MaxPullbackPct:   15.0,
		MaxPullbackDays:  10,
		NearLowPeriod:    60,
		NearLowMaxRatio:  0.15,
	}
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	foundValid := false
	for _, r := range results {
		if r.Valid {
			foundValid = true
			break
		}
	}
	if !foundValid {
		t.Fatal("expected Valid when surge day Open is near low")
	}
}

func TestVolumeSurgeFilterNearLowRejectsSurgeAwayFromBottom(t *testing.T) {
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

	// 底部区域在更早的位置：index 0-5 价格约 5.0
	for i := 0; i <= 5; i++ {
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i+1),
			Open: 5.1, High: 5.3, Low: 4.9, Close: 5.0, Volume: 80000,
		}
	}

	// 放量日 Open 远离底部（10.25 >> 4.9*1.15=5.635）
	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 10.25, High: 10.9, Low: 10.2, Close: 10.86, Volume: 350000,
	}
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 11.0, Low: 10.8, Close: 11.0, Volume: 280000,
	}
	for i := 27; i <= 35; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 40000,
		}
	}

	cfg := VolumeSurgeConfig{
		VolumeMAPeriod:   20,
		MinVolumeRatio:   2.0,
		MinRallyPct:      5.0,
		MaxPullbackPct:   15.0,
		MaxPullbackDays:  10,
		NearLowPeriod:    60,
		NearLowMaxRatio:  0.15,
	}
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	for _, r := range results {
		if r.Valid {
			t.Fatal("expected no Valid when surge day Open is far from low")
		}
	}
}

func TestVolumeSurgeFilterWithSurgeWindowDays(t *testing.T) {
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

	// 第一个放量日
	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 10.25, High: 10.9, Low: 10.2, Close: 10.86, Volume: 350000,
	}
	// 间隔 1 天（非放量日）
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 10.9, Low: 10.6, Close: 10.7, Volume: 80000,
	}
	// 第二个放量日（间隔2天，在 SurgeWindowDays=3 内）
	klines[27] = &model.StockKline{
		Code: "600312", Date: "2026-01-28",
		Open: 10.7, High: 11.5, Low: 10.6, Close: 11.3, Volume: 300000,
	}
	// 峰值
	klines[28] = &model.StockKline{
		Code: "600312", Date: "2026-01-29",
		Open: 11.3, High: 11.5, Low: 11.1, Close: 11.4, Volume: 120000,
	}
	// 回调
	for i := 29; i <= 38; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 40000,
		}
	}

	// SurgeWindowDays=3 应合并两个放量日为一个窗口
	cfg := VolumeSurgeConfig{
		VolumeMAPeriod: 20,
		MinVolumeRatio: 2.0,
		MinRallyPct:    5.0,
		MaxPullbackPct: 15.0,
		MaxPullbackDays: 10,
		SurgeWindowDays: 3,
	}
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	foundValid := false
	for _, r := range results {
		if r.Valid {
			foundValid = true
			break
		}
	}
	if !foundValid {
		t.Fatal("expected Valid with SurgeWindowDays=3 merging surge days")
	}
}

func TestVolumeSurgeFilterWithPullbackToVMARatio(t *testing.T) {
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

	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 10.25, High: 10.9, Low: 10.2, Close: 10.86, Volume: 350000,
	}
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 11.0, Low: 10.8, Close: 11.0, Volume: 280000,
	}
	// 回调期成交量 ~120000（VMA20≈100000，比值1.2 <= 1.5 通过）
	for i := 27; i <= 35; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 120000,
		}
	}

	cfg := VolumeSurgeConfig{
		VolumeMAPeriod:        20,
		MinVolumeRatio:        2.0,
		MinRallyPct:           5.0,
		MaxPullbackPct:        15.0,
		MaxPullbackDays:       10,
		MaxPullbackToVMARatio: 1.5,
	}
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	if !results[30].Valid {
		t.Fatal("expected Valid when pullback volume within VMA20 ratio")
	}
}

func TestVolumeSurgeFilterPullbackToVMARatioTooHigh(t *testing.T) {
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

	klines[25] = &model.StockKline{
		Code: "600312", Date: "2026-01-26",
		Open: 10.25, High: 10.9, Low: 10.2, Close: 10.86, Volume: 350000,
	}
	klines[26] = &model.StockKline{
		Code: "600312", Date: "2026-01-27",
		Open: 10.86, High: 11.0, Low: 10.8, Close: 11.0, Volume: 280000,
	}
	// 回调期成交量 ~250000（VMA20≈100000，比值2.5 > 1.5 不通过）
	for i := 27; i <= 35; i++ {
		prevClose := klines[i-1].Close
		close := prevClose - 0.05
		klines[i] = &model.StockKline{
			Code: "600312", Date: fmt.Sprintf("2026-01-%02d", i-25),
			Open: prevClose, High: prevClose + 0.02, Low: close - 0.02, Close: close, Volume: 250000,
		}
	}

	cfg := VolumeSurgeConfig{
		VolumeMAPeriod:        20,
		MinVolumeRatio:        2.0,
		MinRallyPct:           5.0,
		MaxPullbackPct:        15.0,
		MaxPullbackDays:       10,
		MaxPullbackToVMARatio: 1.5,
	}
	f := NewVolumeSurgeFilter(cfg)
	results := f.Filter(klines)

	if results[30].Valid {
		t.Fatal("expected invalid when pullback volume exceeds VMA20 ratio")
	}
}
