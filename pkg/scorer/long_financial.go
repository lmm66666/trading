package scorer

import (
	"sort"

	"trading/model"
)

// ScoreLongFinancial 财报基本面长线评分（满分 100）
//
// 维度 A：盈利增长（30 分）
// 维度 B：盈利质量（25 分）
// 维度 C：财务安全（25 分）
// 维度 D：运营效率（20 分）
func ScoreLongFinancial(reports []*model.FinancialReport) *ScoreDetail {
	if len(reports) < 4 {
		return &ScoreDetail{Max: 100, Items: nil}
	}

	sorted := make([]*model.FinancialReport, len(reports))
	copy(sorted, reports)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ReportDate < sorted[j].ReportDate
	})

	latest := sorted[len(sorted)-1]
	var items []ScoreItem

	// ── 维度 A：盈利增长（30 分）──

	profitYoY := yoy(sorted, func(r *model.FinancialReport) float64 { return r.NetProfit })
	items = append(items, ScoreItem{
		Name: "净利润同比", Value: profitYoY,
		Score: profitYoYScore(profitYoY), MaxScore: 20,
	})

	revYoY := yoy(sorted, func(r *model.FinancialReport) float64 { return r.TotalRevenue })
	items = append(items, ScoreItem{
		Name: "营收同比", Value: revYoY,
		Score: revYoYScore(revYoY), MaxScore: 10,
	})

	// ── 维度 B：盈利质量（25 分）──

	cfPos := 0
	for _, r := range sorted[len(sorted)-4:] {
		if r.OperatingCashFlow > 0 {
			cfPos++
		}
	}
	items = append(items, ScoreItem{
		Name: "经营现金流(近4期正值)", Value: float64(cfPos),
		Score: cfScore(cfPos), MaxScore: 15,
	})

	gmTrend := trend(sorted, func(r *model.FinancialReport) float64 { return r.GrossMargin / 100 })
	items = append(items, ScoreItem{
		Name: "毛利率趋势", Value: gmTrend,
		Score: trendScore(gmTrend, 0.03, 5, 3), MaxScore: 5,
	})

	roe := latest.ROE / 100
	items = append(items, ScoreItem{
		Name: "ROE", Value: roe,
		Score: roeScore(roe), MaxScore: 5,
	})

	// ── 维度 C：财务安全（25 分）──

	debt := latest.AssetLiabilityRatio / 100
	items = append(items, ScoreItem{
		Name: "资产负债率", Value: debt,
		Score: debtScore(debt), MaxScore: 10,
	})

	cr := latest.CurrentRatio
	items = append(items, ScoreItem{
		Name: "流动比率", Value: cr,
		Score: currentRatioScore(cr), MaxScore: 8,
	})

	qr := latest.QuickRatio
	items = append(items, ScoreItem{
		Name: "速动比率", Value: qr,
		Score: quickRatioScore(qr), MaxScore: 7,
	})

	// ── 维度 D：运营效率（20 分）──

	tatTrend := trend(sorted, func(r *model.FinancialReport) float64 { return r.TotalAssetTurnover })
	items = append(items, ScoreItem{
		Name: "总资产周转率趋势", Value: tatTrend,
		Score: trendScore(tatTrend, 0.1, 8, 5), MaxScore: 8,
	})

	epsScore := epsTrendScore(sorted)
	items = append(items, ScoreItem{
		Name: "EPS增长趋势", Value: 0,
		Score: epsScore, MaxScore: 7,
	})

	cutRatio := 0.0
	if latest.NetProfit != 0 {
		cutRatio = latest.NetProfitCut / latest.NetProfit
	}
	items = append(items, ScoreItem{
		Name: "扣非净利润占比", Value: cutRatio,
		Score: cutRatioScore(cutRatio), MaxScore: 5,
	})

	total := 0
	for _, it := range items {
		total += it.Score
	}
	return &ScoreDetail{Total: total, Max: 100, Items: items}
}

// ── 同比计算 ──

type fieldExtractor func(r *model.FinancialReport) float64

func yoy(reports []*model.FinancialReport, extract fieldExtractor) float64 {
	if len(reports) < 2 {
		return -999
	}
	latest := reports[len(reports)-1]
	yr := yearOf(latest.ReportDate)
	rt := latest.ReportType

	var prev *model.FinancialReport
	for _, r := range reports {
		if yearOf(r.ReportDate) == yr-1 && r.ReportType == rt {
			prev = r
			break
		}
	}
	if prev == nil {
		return -999
	}
	curVal := extract(latest)
	prevVal := extract(prev)
	if prevVal == 0 {
		return -999
	}
	return (curVal - prevVal) / abs(prevVal)
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y := 0
	for _, c := range date[:4] {
		y = y*10 + int(c-'0')
	}
	return y
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// trend 计算近 4 期某字段的趋势（最新 - 最早）
func trend(reports []*model.FinancialReport, extract fieldExtractor) float64 {
	if len(reports) < 4 {
		return -999
	}
	recent := reports[len(reports)-4:]
	return extract(recent[len(recent)-1]) - extract(recent[0])
}

// ── 评分函数 ──

func profitYoYScore(v float64) int {
	switch {
	case v >= 0.3:
		return 20
	case v >= 0.1:
		return 15
	case v >= 0:
		return 8
	case v >= -0.1:
		return 4
	case v >= -0.3:
		return 2
	default:
		return 0
	}
}

func revYoYScore(v float64) int {
	switch {
	case v >= 0.2:
		return 10
	case v >= 0.05:
		return 7
	case v >= 0:
		return 4
	default:
		return 0
	}
}

func cfPos(count int) int {
	if count >= 4 {
		return 15
	}
	if count >= 2 {
		return 8
	}
	return 0
}

func cfScore(count int) int {
	if count >= 4 {
		return 15
	}
	if count >= 2 {
		return 8
	}
	return 0
}

func trendScore(v, threshold float64, high, mid int) int {
	if v >= 0 {
		return high
	}
	if v >= -threshold {
		return mid
	}
	return 0
}

func roeScore(v float64) int {
	if v >= 0.15 {
		return 5
	}
	if v >= 0.05 {
		return 3
	}
	return 0
}

func debtScore(v float64) int {
	if v <= 0.5 {
		return 10
	}
	if v <= 0.7 {
		return 5
	}
	return 0
}

func currentRatioScore(v float64) int {
	if v >= 2.0 {
		return 8
	}
	if v >= 1.0 {
		return 5
	}
	return 0
}

func quickRatioScore(v float64) int {
	if v >= 1.0 {
		return 7
	}
	if v >= 0.5 {
		return 4
	}
	return 0
}

func epsTrendScore(reports []*model.FinancialReport) int {
	if len(reports) < 4 {
		return 0
	}
	recent := reports[len(reports)-4:]
	increasing := true
	decreasing := true
	for i := 0; i < len(recent)-1; i++ {
		if recent[i].EPS > recent[i+1].EPS {
			increasing = false
		}
		if recent[i].EPS < recent[i+1].EPS {
			decreasing = false
		}
	}
	if increasing {
		return 7
	}
	if decreasing {
		return 0
	}
	return 4
}

func cutRatioScore(v float64) int {
	if v >= 0.9 {
		return 5
	}
	if v >= 0.7 {
		return 3
	}
	return 0
}
