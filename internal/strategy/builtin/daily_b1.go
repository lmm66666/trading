package builtin

import (
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

const (
	strategyVersion = "1"
	dailyB1ID       = "daily_b1_buy"

	dailyVolumeRatioParam     = "volume_ratio"
	dailyRallyPctParam        = "rally_pct"
	dailyPullbackPctParam     = "pullback_pct"
	dailyPullbackBarsParam    = "pullback_bars"
	dailyKDJThresholdParam    = "kdj_threshold"
	dailyMATrendLookbackParam = "ma_trend_lookback"
)

var (
	dailyCloseRef      = indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close}
	dailyVolumeRef     = indicator.Ref{Kind: indicator.VolumeMA, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Volume, Period: 1}
	dailyVolumeMA20Ref = indicator.Ref{Kind: indicator.VolumeMA, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Volume, Period: 20}
	dailyKDJJRef       = indicator.Ref{Kind: indicator.KDJKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.J, Period: 9}
	dailyMA20Ref       = indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close, Period: 20}
)

type dailyB1 struct {
	params  map[string]float64
	tracker *pullbackTracker
}

func NewDailyB1(params map[string]float64) (strategy.Strategy, error) {
	resolved := dailyParameters(params)
	return &dailyB1{
		params: resolved,
		tracker: newPullbackTracker(trackerConfig{
			MaxPullbackPct:  resolved[dailyPullbackPctParam],
			MaxPullbackBars: int(resolved[dailyPullbackBarsParam]),
		}),
	}, nil
}

func (s *dailyB1) Definition() strategy.Definition {
	return strategy.Definition{
		ID: dailyB1ID, Version: strategyVersion, PrimaryTimeframe: market.Day,
		WarmupBars: 30, DefaultHoldBars: 10,
		Features:   []indicator.Ref{dailyCloseRef, dailyVolumeRef, dailyVolumeMA20Ref, dailyKDJJRef, dailyMA20Ref},
		Parameters: dailyParameterSpecs(),
	}
}

func (s *dailyB1) OnBar(context strategy.Context) (strategy.Decision, error) {
	close, closeOK := context.Float(dailyCloseRef, 0)
	previousClose, previousOK := context.Float(dailyCloseRef, 1)
	volume, volumeValueOK := context.Float(dailyVolumeRef, 0)
	volumeMA, volumeMAOK := context.Float(dailyVolumeMA20Ref, 0)
	if !closeOK || !previousOK || !volumeValueOK || !volumeMAOK || volumeMA <= 0 {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	surge := volume/volumeMA >= s.params[dailyVolumeRatioParam] &&
		percentageChange(close, previousClose) >= s.params[dailyRallyPctParam]
	tracked := s.tracker.Advance(trackerInput{Index: context.Index(), Close: close, Surge: surge})
	if !tracked.Signal || !dailyMATrendingUp(context, int(s.params[dailyMATrendLookbackParam])) {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	jd, jOK := context.Float(dailyKDJJRef, 0)
	if !jOK || jd >= s.params[dailyKDJThresholdParam] {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	return strategy.Decision{Action: strategy.EnterLong, Reason: "daily_b1_pullback"}, nil
}

func dailyMATrendingUp(context strategy.Context, lookback int) bool {
	for ago := 0; ago < lookback; ago++ {
		current, currentOK := context.Float(dailyMA20Ref, ago)
		previous, previousOK := context.Float(dailyMA20Ref, ago+1)
		if !currentOK || !previousOK || current <= previous {
			return false
		}
	}
	return true
}

func dailyParameters(params map[string]float64) map[string]float64 {
	return resolvedParameters(dailyParameterSpecs(), params)
}

func dailyParameterSpecs() map[string]strategy.ParameterSpec {
	return map[string]strategy.ParameterSpec{
		dailyVolumeRatioParam:     {Default: 2, Min: 0.01, Max: 100},
		dailyRallyPctParam:        {Default: 5, Min: 0, Max: 1_000},
		dailyPullbackPctParam:     {Default: 15, Min: 0, Max: 100},
		dailyPullbackBarsParam:    {Default: 10, Min: 1, Max: 365, Integer: true},
		dailyKDJThresholdParam:    {Default: 40, Min: -100, Max: 200},
		dailyMATrendLookbackParam: {Default: 10, Min: 1, Max: 10, Integer: true},
	}
}
