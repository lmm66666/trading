package scorer

import (
	"trading/model"
	"trading/pkg/indicator"
)

// ScoreBottomSurgeShort 底部倍量回调短线评分（满分 100）
//
// 维度 A：放量拉升强度（50 分）
// 维度 B：缩量回调质量（50 分）
func ScoreBottomSurgeShort(klines []*model.StockKline) *ScoreDetail {
	n := len(klines)
	if n < 25 {
		return &ScoreDetail{Max: 100, Items: nil}
	}

	volumes := make([]int64, n)
	closes := make([]float64, n)
	for i, k := range klines {
		volumes[i] = k.Volume
		closes[i] = k.Close
	}
	vma := indicator.ComputeMA(volumes, 20)
	ma20 := indicator.ComputeMA(closes, 20)

	window := findSurgeWindow(klines, volumes, vma)
	if window == nil {
		return &ScoreDetail{Max: 100, Items: nil}
	}

	var items []ScoreItem

	// ── 维度 A：放量拉升强度（50 分）──

	// 单日最大量比（20 分）
	maxRatio := 0.0
	for i := window.surgeStart; i <= window.peakIdx; i++ {
		if vma[i] > 0 {
			ratio := float64(volumes[i]) / vma[i]
			if ratio > maxRatio {
				maxRatio = ratio
			}
		}
	}
	items = append(items, ScoreItem{
		Name: "单日最大量比", Value: maxRatio,
		Score: stepScore(maxRatio, 5, 3, 2), MaxScore: 20,
	})

	// 拉升累计涨幅（15 分）
	base := closes[0]
	if window.surgeStart > 0 {
		base = closes[window.surgeStart-1]
	}
	rallyPct := (window.peakPrice - base) / base * 100
	items = append(items, ScoreItem{
		Name: "拉升累计涨幅", Value: rallyPct,
		Score: stepScore(rallyPct, 30, 20, 10), MaxScore: 15,
	})

	// 连阳天数（10 分）
	maxStreak := 0
	streak := 0
	for i := window.surgeStart; i <= window.peakIdx; i++ {
		if i > 0 && closes[i] > closes[i-1] {
			streak++
			if streak > maxStreak {
				maxStreak = streak
			}
		} else {
			streak = 0
		}
	}
	items = append(items, ScoreItem{
		Name: "连阳天数", Value: float64(maxStreak),
		Score: intScore(maxStreak, 3, 2), MaxScore: 10,
	})

	// 大阳线占比（5 分）
	totalDays := window.peakIdx - window.surgeStart + 1
	yangCount := 0
	for i := window.surgeStart; i <= window.peakIdx; i++ {
		if klines[i].Close > klines[i].Open {
			yangCount++
		}
	}
	yangRatio := float64(yangCount) / float64(totalDays)
	items = append(items, ScoreItem{
		Name: "大阳线占比", Value: yangRatio,
		Score: ratioScore(yangRatio, 0.7, 0.5, 5, 3), MaxScore: 5,
	})

	// ── 维度 B：缩量回调质量（50 分）──

	// 缩量率（20 分）
	surgePeakVol := int64(0)
	for i := window.surgeStart; i <= window.peakIdx; i++ {
		if volumes[i] > surgePeakVol {
			surgePeakVol = volumes[i]
		}
	}
	pbDays := window.pullbackEnd - window.pullbackStart + 1
	var pbVolSum int64
	for i := window.pullbackStart; i <= window.pullbackEnd; i++ {
		pbVolSum += volumes[i]
	}
	shrinkRate := 0.0
	if surgePeakVol > 0 && pbDays > 0 {
		shrinkRate = 1.0 - float64(pbVolSum)/float64(pbDays)/float64(surgePeakVol)
	}
	items = append(items, ScoreItem{
		Name: "缩量率", Value: shrinkRate,
		Score: stepScore(shrinkRate, 0.7, 0.5, 0.3), MaxScore: 20,
	})

	// 回调深度（15 分）
	latestClose := closes[window.pullbackEnd]
	pullbackPct := (window.peakPrice - latestClose) / window.peakPrice * 100
	items = append(items, ScoreItem{
		Name: "回调深度", Value: pullbackPct,
		Score: inverseStepScore(pullbackPct, 5, 10, 15), MaxScore: 15,
	})

	// MA20 支撑（10 分）
	latestMA20 := ma20[window.pullbackEnd]
	allAbove := true
	anyBelow := false
	for i := window.pullbackStart; i <= window.pullbackEnd; i++ {
		if ma20[i] > 0 {
			if klines[i].Close < ma20[i]*0.98 {
				allAbove = false
				anyBelow = true
			}
		}
	}
	ma20Score := 0
	if latestClose >= latestMA20 && allAbove {
		ma20Score = 10
	} else if latestClose >= latestMA20 || !anyBelow {
		ma20Score = 5
	}
	deviation := 0.0
	if latestMA20 > 0 {
		deviation = (latestClose - latestMA20) / latestMA20 * 100
	}
	items = append(items, ScoreItem{
		Name: "MA20支撑", Value: deviation,
		Score: ma20Score, MaxScore: 10,
	})

	// 整理纯净度（5 分）
	clean := true
	for i := window.pullbackStart; i <= window.pullbackEnd; i++ {
		if klines[i].Close < klines[i].Open && volumes[i] >= surgePeakVol {
			clean = false
			break
		}
	}
	cleanVal := 1.0
	cleanScore := 5
	if !clean {
		cleanVal = 0
		cleanScore = 0
	}
	items = append(items, ScoreItem{
		Name: "整理纯净度", Value: cleanVal,
		Score: cleanScore, MaxScore: 5,
	})

	total := 0
	for _, it := range items {
		total += it.Score
	}
	return &ScoreDetail{Total: total, Max: 100, Items: items}
}

// ── 拉升窗口识别 ──

type surgeWindow struct {
	surgeStart    int
	peakIdx       int
	peakPrice     float64
	pullbackStart int
	pullbackEnd   int
}

func findSurgeWindow(klines []*model.StockKline, volumes []int64, vma []float64) *surgeWindow {
	n := len(klines)

	// 找所有量比 >= 2.0 的放量日
	var surgeDays []int
	for i := 20; i < n; i++ {
		if vma[i] > 0 && float64(volumes[i])/vma[i] >= 2.0 {
			surgeDays = append(surgeDays, i)
		}
	}
	if len(surgeDays) == 0 {
		return nil
	}

	// 从后往前找拉升窗口
	for si := len(surgeDays) - 1; si >= 0; si-- {
		start := surgeDays[si]

		// 允许 3 天间隔内的多个放量日
		lastSurge := start
		for j := start + 1; j < n && j-lastSurge <= 3; j++ {
			if vma[j] > 0 && float64(volumes[j])/vma[j] >= 1.2 {
				lastSurge = j
			}
		}

		// 找峰值：从 lastSurge 开始连续收盘价上涨
		peakIdx := lastSurge
		peakPrice := klines[lastSurge].Close
		for j := lastSurge + 1; j < n; j++ {
			if klines[j].Close >= peakPrice {
				peakIdx = j
				peakPrice = klines[j].Close
			} else {
				break
			}
		}

		// 峰值之后需要有回调数据
		if peakIdx >= n-1 {
			continue
		}

		return &surgeWindow{
			surgeStart:    start,
			peakIdx:       peakIdx,
			peakPrice:     peakPrice,
			pullbackStart: peakIdx + 1,
			pullbackEnd:   n - 1,
		}
	}
	return nil
}

// ── 评分工具函数 ──

// stepScore 阶梯评分：value >= t1 得 max, >= t2 得 max*3/4, >= t3 得 max/2, else 0
func stepScore(value, t1, t2, t3 float64) int {
	switch {
	case value >= t1:
		return 20
	case value >= t2:
		return 15
	case value >= t3:
		return 10
	default:
		return 0
	}
}

// inverseStepScore 反向阶梯评分：value <= t1 得 15, <= t2 得 10, <= t3 得 5, else 0
func inverseStepScore(value, t1, t2, t3 float64) int {
	switch {
	case value <= t1:
		return 15
	case value <= t2:
		return 10
	case value <= t3:
		return 5
	default:
		return 0
	}
}

// intScore 整数阶梯评分：v >= h 得 high, v >= m 得 mid, else 0
func intScore(v, h, m int) int {
	switch {
	case v >= h:
		return 10
	case v >= m:
		return 5
	default:
		return 0
	}
}

// ratioScore 比率阶梯评分：v >= h 得 high, v >= m 得 mid, else 0
func ratioScore(v, h, m float64, high, mid int) int {
	switch {
	case v >= h:
		return high
	case v >= m:
		return mid
	default:
		return 0
	}
}
