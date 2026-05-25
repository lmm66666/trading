package scorer

import (
	"fmt"
	"testing"

	"trading/model"
)

// ── 构建底部倍量 + 缩量回调的 K 线序列 ──

func buildBottomSurgeKlines() []*model.StockKline {
	var klines []*model.StockKline
	day := 1

	// 阶段1：底部横盘 20 天（低量、窄幅）
	for range 20 {
		klines = append(klines, makeKline("600000", fmt.Sprintf("2026-01-%02d", day), 10.0, 10.2, 9.8, 10.0, 10000))
		day++
	}

	// 阶段2：放量拉升 5 天（量比 5x，阳线为主）
	prices := []float64{10.3, 10.8, 11.5, 12.2, 13.0}
	for _, p := range prices {
		klines = append(klines, makeKline("600000", fmt.Sprintf("2026-01-%02d", day), p-0.1, p+0.2, p-0.2, p, 50000))
		day++
	}

	// 阶段3：缩量回调 10 天（量能萎缩到峰值 ~20%）
	for i := range 10 {
		closeP := 12.5 - float64(i)*0.05
		klines = append(klines, makeKline("600000", fmt.Sprintf("2026-01-%02d", day), closeP-0.1, closeP+0.1, closeP-0.15, closeP, 8000))
		day++
	}

	return klines
}

func TestScoreBottomSurgeShort_InsufficientData(t *testing.T) {
	klines := make([]*model.StockKline, 10)
	for i := range klines {
		klines[i] = makeKline("600000", fmt.Sprintf("2026-01-%02d", i+1), 10, 10.5, 9.5, 10, 10000)
	}
	result := ScoreBottomSurgeShort(klines)
	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items should be empty for insufficient data, got %d items", len(result.Items))
	}
}

func TestScoreBottomSurgeShort_NoSurgeWindow(t *testing.T) {
	n := 40
	klines := make([]*model.StockKline, n)
	for i := 0; i < n; i++ {
		klines[i] = makeKline("600000", fmt.Sprintf("2026-01-%02d", i+1), 10, 10.2, 9.8, 10, 10000)
	}
	result := ScoreBottomSurgeShort(klines)
	if len(result.Items) != 0 {
		t.Errorf("Items should be empty when no surge window found, got %d items", len(result.Items))
	}
}

func TestScoreBottomSurgeShort_IdealCase(t *testing.T) {
	klines := buildBottomSurgeKlines()
	result := ScoreBottomSurgeShort(klines)

	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	if len(result.Items) == 0 {
		t.Fatal("Items should not be empty for ideal bottom surge case")
	}
	if result.Total < 30 {
		t.Errorf("Total = %d, too low for ideal bottom surge case", result.Total)
	}
	if result.Total > 100 {
		t.Errorf("Total = %d, exceeds max 100", result.Total)
	}

	// 验证各维度 MaxScore 合计 = 100
	maxSum := 0
	for _, item := range result.Items {
		maxSum += item.MaxScore
		if item.Score < 0 || item.Score > item.MaxScore {
			t.Errorf("item %q: Score=%d out of range [0, %d]", item.Name, item.Score, item.MaxScore)
		}
	}
	if maxSum != 100 {
		t.Errorf("MaxScore sum = %d, want 100", maxSum)
	}

	// 验证关键评分项
	foundSurgeRatio := false
	foundShrinkRate := false
	for _, item := range result.Items {
		if item.Name == "单日最大量比" {
			foundSurgeRatio = true
			if item.Value < 2.0 {
				t.Errorf("单日最大量比 = %.2f, should be >= 2.0 for surge case", item.Value)
			}
		}
		if item.Name == "缩量率" {
			foundShrinkRate = true
			if item.Value < 0.3 {
				t.Errorf("缩量率 = %.2f, should be higher for pullback case", item.Value)
			}
		}
	}
	if !foundSurgeRatio {
		t.Error("missing '单日最大量比' item")
	}
	if !foundShrinkRate {
		t.Error("missing '缩量率' item")
	}
}

func TestScoreBottomSurgeShort_ScoreBounds(t *testing.T) {
	klines := buildBottomSurgeKlines()
	result := ScoreBottomSurgeShort(klines)

	for _, item := range result.Items {
		if item.Score < 0 {
			t.Errorf("item %q: negative score %d", item.Name, item.Score)
		}
		if item.Score > item.MaxScore {
			t.Errorf("item %q: score %d exceeds max %d", item.Name, item.Score, item.MaxScore)
		}
	}
}

func TestFindSurgeWindow_NoSurge(t *testing.T) {
	n := 30
	klines := make([]*model.StockKline, n)
	volumes := make([]int64, n)
	for i := 0; i < n; i++ {
		klines[i] = makeKline("600000", fmt.Sprintf("2026-01-%02d", i+1), 10, 10.2, 9.8, 10, 10000)
		volumes[i] = 10000
	}
	vma := make([]float64, n)

	w := findSurgeWindow(klines, volumes, vma)
	if w != nil {
		t.Error("expected nil surge window when no volume surge")
	}
}

func TestFindSurgeWindow_WithSurge(t *testing.T) {
	n := 40
	klines := make([]*model.StockKline, n)
	volumes := make([]int64, n)
	vma := make([]float64, n)

	for i := 0; i < n; i++ {
		vol := int64(10000)
		closeP := 10.0
		if i >= 25 && i <= 28 {
			vol = 50000
			closeP = 10.0 + float64(i-25)*0.5
		}
		if i > 28 {
			vol = 5000
			closeP = 12.0 - float64(i-28)*0.1
		}
		klines[i] = makeKline("600000", fmt.Sprintf("2026-01-%02d", i+1), closeP-0.1, closeP+0.1, closeP-0.15, closeP, vol)
		volumes[i] = vol
		if i >= 20 {
			vma[i] = 10000.0
		}
	}

	w := findSurgeWindow(klines, volumes, vma)
	if w == nil {
		t.Fatal("expected non-nil surge window")
	}
	if w.surgeStart < 20 {
		t.Errorf("surgeStart = %d, should be >= 20", w.surgeStart)
	}
	if w.pullbackEnd != n-1 {
		t.Errorf("pullbackEnd = %d, want %d", w.pullbackEnd, n-1)
	}
}
