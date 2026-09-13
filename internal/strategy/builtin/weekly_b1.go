package builtin

import (
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

const (
	weeklyB1ID              = "weekly_b1_buy"
	weeklyKDJThresholdParam = "kdj_threshold"
)

var (
	weeklyCloseRef = indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Week, PriceView: market.ForwardAdjusted, Field: indicator.Close}
	weeklyKDJJRef  = indicator.Ref{Kind: indicator.KDJKind, Timeframe: market.Week, PriceView: market.ForwardAdjusted, Field: indicator.J, Period: 9}
	weeklyMA20Ref  = indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Week, PriceView: market.ForwardAdjusted, Field: indicator.Close, Period: 20}
	weeklyMA60Ref  = indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Week, PriceView: market.ForwardAdjusted, Field: indicator.Close, Period: 60}
)

type weeklyB1 struct{ params map[string]float64 }

func NewWeeklyB1(params map[string]float64) (strategy.Strategy, error) {
	return &weeklyB1{params: resolvedParameters(weeklyParameterSpecs(), params)}, nil
}

func (s *weeklyB1) Definition() strategy.Definition {
	return strategy.Definition{
		ID: weeklyB1ID, Version: strategyVersion, PrimaryTimeframe: market.Week,
		WarmupBars: 60, DefaultHoldBars: 10,
		Features:  []indicator.Ref{weeklyCloseRef, weeklyKDJJRef, weeklyMA20Ref, weeklyMA60Ref, dailyMA20Ref},
		Auxiliary: []market.Timeframe{market.Day}, Parameters: weeklyParameterSpecs(),
	}
}

func (s *weeklyB1) OnBar(context strategy.Context) (strategy.Decision, error) {
	jd, jOK := context.Float(weeklyKDJJRef, 0)
	ma20, ma20OK := context.Float(weeklyMA20Ref, 0)
	ma60, ma60OK := context.Float(weeklyMA60Ref, 0)
	close, closeOK := context.Float(weeklyCloseRef, 0)
	dailyMA20, dailyOK := context.Float(dailyMA20Ref, 0)
	if !jOK || !ma20OK || !ma60OK || !closeOK || !dailyOK ||
		jd >= s.params[weeklyKDJThresholdParam] || ma20 <= ma60 || close < ma60 || close < dailyMA20 {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	return strategy.Decision{Action: strategy.EnterLong, Reason: "weekly_b1_alignment"}, nil
}

func weeklyParameterSpecs() map[string]strategy.ParameterSpec {
	return map[string]strategy.ParameterSpec{
		weeklyKDJThresholdParam: {Default: 10, Min: -100, Max: 200},
	}
}
