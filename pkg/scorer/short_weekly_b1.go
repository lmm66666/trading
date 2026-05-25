package scorer

import (
	"trading/model"
	"trading/pkg/indicator"
)

// ScoreWeeklyB1Short 周线 B1 短线评分（满分 100）
//
// 维度 A：KDJ 超卖程度（40 分）
// 维度 B：均线多头排列（30 分）
// 维度 C：跨周期确认（30 分）
func ScoreWeeklyB1Short(weekly []*model.StockKline, daily []*model.StockKline) *ScoreDetail {
	n := len(weekly)
	if n < 5 {
		return &ScoreDetail{Max: 100, Items: nil}
	}

	closes := make([]float64, n)
	for i, k := range weekly {
		closes[i] = k.Close
	}
	ma20 := indicator.ComputeMA(closes, 20)
	ma60 := indicator.ComputeMA(closes, 60)
	kdj := indicator.ComputeKDJ(weekly)

	latest := weekly[n-1]
	jVal := kdj[n-1].J
	kVal := kdj[n-1].K
	dVal := kdj[n-1].D
	latestClose := latest.Close
	latestMA20 := ma20[n-1]
	latestMA60 := ma60[n-1]

	var items []ScoreItem

	// ── 维度 A：KDJ 超卖程度（40 分）──

	items = append(items, ScoreItem{
		Name: "J值位置", Value: jVal,
		Score: inverseRangeScore(jVal, 0, 10, 20, 20, 15, 10), MaxScore: 20,
	})

	items = append(items, ScoreItem{
		Name: "K/D同步低位", Value: max(kVal, dVal),
		Score: kdSyncScore(kVal, dVal), MaxScore: 10,
	})

	oversoldWeeks := 0
	for i := len(kdj) - 1; i >= 0; i-- {
		if kdj[i].J < 10 {
			oversoldWeeks++
		} else {
			break
		}
	}
	items = append(items, ScoreItem{
		Name: "超卖持续周数", Value: float64(oversoldWeeks),
		Score: intScore(oversoldWeeks, 3, 1), MaxScore: 10,
	})

	// ── 维度 B：均线多头排列（30 分）──

	slopeUp := n >= 3 && latestMA20 > ma20[n-3]
	maCrossScore := 0
	if latestMA20 > latestMA60 {
		if slopeUp {
			maCrossScore = 10
		} else {
			maCrossScore = 5
		}
	}
	ma20Ratio := 0.0
	if latestMA60 > 0 {
		ma20Ratio = latestMA20 / latestMA60
	}
	items = append(items, ScoreItem{
		Name: "MA20>MA60", Value: ma20Ratio,
		Score: maCrossScore, MaxScore: 10,
	})

	items = append(items, ScoreItem{
		Name: "股价vsMA20", Value: closeMAPct(latestClose, latestMA20),
		Score: closeMAScore(latestClose, latestMA20), MaxScore: 10,
	})

	items = append(items, ScoreItem{
		Name: "股价vsMA60", Value: closeMAPct(latestClose, latestMA60),
		Score: closeMAScore(latestClose, latestMA60), MaxScore: 10,
	})

	// ── 维度 C：跨周期确认（30 分）──

	dailyScore := 0
	dailyDev := -999.0
	if len(daily) >= 20 {
		dCloses := make([]float64, len(daily))
		for i, k := range daily {
			dCloses[i] = k.Close
		}
		dMA20 := indicator.ComputeMA(dCloses, 20)
		dClose := daily[len(daily)-1].Close
		dMA20Val := dMA20[len(dMA20)-1]
		if dMA20Val > 0 {
			dailyDev = (dClose - dMA20Val) / dMA20Val * 100
		}
		dailyScore = closeMAScoreWithMiddle(dClose, dMA20Val, 15, 8)
	}
	items = append(items, ScoreItem{
		Name: "日线MA20支撑", Value: dailyDev,
		Score: dailyScore, MaxScore: 15,
	})

	highPrice := weekly[0].High
	for _, k := range weekly {
		if k.High > highPrice {
			highPrice = k.High
		}
	}
	adjustPct := (highPrice - latestClose) / highPrice * 100
	items = append(items, ScoreItem{
		Name: "调整幅度", Value: adjustPct,
		Score: inverseStepScore(adjustPct, 15, 25, 100), MaxScore: 10,
	})

	highIdx := 0
	for i := 1; i < n; i++ {
		if weekly[i].High > weekly[highIdx].High {
			highIdx = i
		}
	}
	adjustWeeks := n - 1 - highIdx
	items = append(items, ScoreItem{
		Name: "调整周期", Value: float64(adjustWeeks),
		Score: adjustWeeksScore(adjustWeeks), MaxScore: 5,
	})

	total := 0
	for _, it := range items {
		total += it.Score
	}
	return &ScoreDetail{Total: total, Max: 100, Items: items}
}

// ── 评分工具函数 ──

func inverseRangeScore(val, t1, t2, t3 float64, s1, s2, s3 int) int {
	switch {
	case val < t1:
		return s1
	case val <= t2:
		return s2
	case val <= t3:
		return s3
	default:
		return 0
	}
}

func kdSyncScore(k, d float64) int {
	if k < 20 && d < 20 {
		return 10
	}
	if k <= 30 || d <= 30 {
		return 5
	}
	return 0
}

func closeMAPct(close, ma float64) float64 {
	if ma > 0 {
		return (close - ma) / ma * 100
	}
	return -999
}

func closeMAScore(close, ma float64) int {
	if ma <= 0 {
		return 0
	}
	if close > ma {
		return 10
	}
	if close >= ma*0.98 {
		return 5
	}
	return 0
}

func closeMAScoreWithMiddle(close, ma float64, high, mid int) int {
	if ma <= 0 {
		return 0
	}
	if close > ma {
		return high
	}
	if close >= ma*0.98 {
		return mid
	}
	return 0
}

func adjustWeeksScore(weeks int) int {
	switch {
	case weeks <= 8:
		return 5
	case weeks <= 16:
		return 3
	default:
		return 0
	}
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
