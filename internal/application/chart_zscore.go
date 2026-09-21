package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
	"trading/internal/indicator"
	"trading/internal/market"
)

type ZScoreDiagnostic struct {
	Key                 string   `json:"key"`
	Comparison          string   `json:"comparison"`
	Period              int      `json:"period"`
	Regime              int      `json:"regime"`
	Lag                 int      `json:"lag"`
	CommodityDate       string   `json:"commodity_date"`
	Z                   *float64 `json:"z"`
	LongZ               *float64 `json:"long_z"`
	RelativePerformance *float64 `json:"relative_performance"`
	Correlation         *float64 `json:"correlation"`
	State               string   `json:"state"`
	Warning             string   `json:"warning,omitempty"`
}

func (s *ChartQueryService) addZScores(ctx context.Context, q ChartQuery, stock market.Dataset, factors []market.AdjustmentFactor, from, to time.Time, start int, result *ChartResult) error {
	requests := []IndicatorRequest{}
	for _, r := range q.Indicators {
		if r.Kind == IndicatorZSCORE {
			requests = append(requests, r)
		}
	}
	if len(requests) == 0 {
		return nil
	}
	warning := ""
	var commodity market.Dataset
	switch {
	case q.Timeframe != market.Day:
		warning = "Z-score 仅支持日线"
	case q.Comparison == "":
		warning = "请选择看板关联期货后计算 Z-score"
	default:
		id, _ := market.ParseInstrumentID(q.Comparison)
		var err error
		commodity, _, _, err = s.data.Dataset(ctx, id, market.Day, from, to, q.DataVersion)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		switch {
		case err != nil:
			warning = "关联期货读取失败，请重试"
		case commodity.Instrument() != id || commodity.Timeframe() != market.Day || commodity.Version() != q.DataVersion:
			warning = "关联期货数据版本不一致"
		case commodity.Len() == 0:
			warning = "关联期货暂无历史数据"
		}
	}
	for _, r := range requests {
		key := zScoreKey(q, r, "histogram")
		diagnostic := ZScoreDiagnostic{Key: key, Comparison: q.Comparison, Period: r.Period, Regime: r.Regime, Lag: r.Lag, Warning: warning, State: "数据不足"}
		if warning != "" {
			result.ZScores = append(result.ZScores, diagnostic)
			continue
		}
		primary, secondary := []float64{}, []float64{}
		dates := []time.Time{}
		offset := 0
		cursor := -1
		factorIndex := 0
		for i := 0; i < stock.Len(); i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			bar := stock.Bar(i)
			day := bar.CloseTime.UTC().Format("2006-01-02")
			for cursor+1 < commodity.Len() && commodity.Bar(cursor+1).CloseTime.UTC().Format("2006-01-02") <= day {
				cursor++
			}
			aligned := cursor - r.Lag
			if aligned < 0 {
				offset = i + 1
				continue
			}
			price, err := chartPriceBar(bar, q.View, factors, &factorIndex)
			if err != nil {
				return err
			}
			primary = append(primary, price.Close)
			secondary = append(secondary, float64(commodity.Bar(aligned).Close)/float64(market.ValueScale))
			dates = append(dates, commodity.Bar(aligned).CloseTime)
		}
		panel, err := indicator.RelativePanel(primary, secondary, r.Period, r.Smooth, r.Regime)
		if err != nil {
			return err
		}
		for _, component := range []struct {
			name   string
			values indicator.Series
		}{{"histogram", panel.Z}, {"smooth", panel.Smooth}, {"regime", panel.Regime}} {
			series := ChartSeries{Key: zScoreKey(q, r, component.name), Kind: IndicatorZSCORE, Component: component.name, Points: []ChartPoint{}}
			for j := max(0, start-offset); j < len(primary); j++ {
				if v, valid := component.values.At(j); valid {
					series.Points = append(series.Points, ChartPoint{Time: stock.Bar(j + offset).CloseTime.UTC(), Value: v})
				}
			}
			result.Series = append(result.Series, series)
		}
		if len(primary) > 0 {
			last := len(primary) - 1
			diagnostic.CommodityDate = dates[last].UTC().Format("2006-01-02")
			diagnostic.Z = seriesLast(panel.Z, last)
			diagnostic.LongZ = seriesLast(panel.Regime, last)
			diagnostic.RelativePerformance = seriesLast(panel.Performance, last)
			diagnostic.Correlation = seriesLast(panel.Correlation, last)
			diagnostic.State = zScoreState(diagnostic.Z, diagnostic.LongZ)
		}
		result.ZScores = append(result.ZScores, diagnostic)
	}
	return ctx.Err()
}
func seriesLast(s indicator.Series, i int) *float64 {
	v, ok := s.At(i)
	if !ok {
		return nil
	}
	return &v
}
func zScoreState(z, long *float64) string {
	if z == nil {
		return "数据不足"
	}
	if long != nil && math.Abs(*z) >= 2 && math.Abs(*long) >= 2 && *z**long > 0 {
		return "长期偏离，核查结构变化"
	}
	if *z >= 2 {
		return "股票阶段性偏强"
	}
	if *z <= -2 {
		return "股票阶段性偏弱"
	}
	if math.Abs(*z) <= .75 {
		return "常态区"
	}
	return "偏离观察"
}

func zScoreKey(q ChartQuery, r IndicatorRequest, component string) string {
	view := "raw"
	if q.View == market.ForwardAdjusted {
		view = "qfq"
	}
	return fmt.Sprintf("zscore/day/%s/%s/p=%d/sm=%d/rg=%d/lag=%d/%s/%s", view, component, r.Period, r.Smooth, r.Regime, r.Lag, q.Instrument, q.Comparison)
}
