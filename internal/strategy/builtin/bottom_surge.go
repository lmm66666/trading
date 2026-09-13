package builtin

import (
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

const (
	bottomSurgeID            = "bottom_surge_pullback"
	bottomLowBandParam       = "low_band_pct"
	bottomSingleVolumeParam  = "single_volume_ratio"
	bottomSingleRallyParam   = "single_rally_pct"
	bottomGradualDaysParam   = "gradual_days"
	bottomGradualVolumeParam = "gradual_volume_ratio"
	bottomGradualRallyParam  = "gradual_rally_pct"
	bottomSurgeGapParam      = "surge_gap"
	bottomPullbackPctParam   = "pullback_pct"
	bottomPullbackBarsParam  = "pullback_bars"
	bottomJMinParam          = "j_min"
	bottomJMaxParam          = "j_max"
)

var (
	bottomOpenRef = indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Open}
	bottomLowRef  = indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Low}
	bottomMA60Ref = indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close, Period: 60}
)

type bottomSurge struct {
	params  map[string]float64
	tracker *pullbackTracker
}

func NewBottomSurge(params map[string]float64) (strategy.Strategy, error) {
	resolved := resolvedParameters(bottomParameterSpecs(), params)
	return &bottomSurge{params: resolved, tracker: newPullbackTracker(trackerConfig{
		MaxPullbackPct: resolved[bottomPullbackPctParam], MaxPullbackBars: int(resolved[bottomPullbackBarsParam]),
		SurgeGap: int(resolved[bottomSurgeGapParam]), GradualDays: int(resolved[bottomGradualDaysParam]), RequireNearLow: true,
	})}, nil
}

func (s *bottomSurge) Definition() strategy.Definition {
	return strategy.Definition{
		ID: bottomSurgeID, Version: strategyVersion, PrimaryTimeframe: market.Day,
		WarmupBars: 60, DefaultHoldBars: 10,
		Features:   []indicator.Ref{bottomOpenRef, bottomLowRef, dailyCloseRef, dailyVolumeRef, dailyVolumeMA20Ref, dailyKDJJRef, dailyMA20Ref, bottomMA60Ref},
		Parameters: bottomParameterSpecs(),
	}
}

func (s *bottomSurge) OnBar(context strategy.Context) (strategy.Decision, error) {
	close, closeOK := context.Float(dailyCloseRef, 0)
	previousClose, previousOK := context.Float(dailyCloseRef, 1)
	volume, volumeValueOK := context.Float(dailyVolumeRef, 0)
	volumeMA, volumeMAOK := context.Float(dailyVolumeMA20Ref, 0)
	if !closeOK || !previousOK || !volumeValueOK || !volumeMAOK || volumeMA <= 0 {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	volumeRatio := volume / volumeMA
	rallyPct := percentageChange(close, previousClose)
	nearLow := isNearSixtyBarLow(context, s.params[bottomLowBandParam])
	tracked := s.tracker.Advance(trackerInput{
		Index: context.Index(), Close: close,
		Surge:   volumeRatio >= s.params[bottomSingleVolumeParam] && rallyPct >= s.params[bottomSingleRallyParam],
		Gradual: volumeRatio >= s.params[bottomGradualVolumeParam] && rallyPct >= s.params[bottomGradualRallyParam],
		NearLow: nearLow,
	})
	if !tracked.Signal {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	ma20, ma20OK := context.Float(dailyMA20Ref, 0)
	ma60, ma60OK := context.Float(bottomMA60Ref, 0)
	jd, jOK := context.Float(dailyKDJJRef, 0)
	if !ma20OK || !ma60OK || !jOK || ma20 <= ma60 || close < ma60 || jd < s.params[bottomJMinParam] || jd > s.params[bottomJMaxParam] {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	return strategy.Decision{Action: strategy.EnterLong, Reason: "bottom_surge_pullback"}, nil
}

func isNearSixtyBarLow(context strategy.Context, bandPct float64) bool {
	open, openOK := context.Float(bottomOpenRef, 0)
	if !openOK {
		return false
	}
	var low float64
	for ago := 0; ago < 60; ago++ {
		candidate, candidateOK := context.Float(bottomLowRef, ago)
		if !candidateOK {
			return false
		}
		if ago == 0 || candidate < low {
			low = candidate
		}
	}
	return open <= low*(1+bandPct/100)
}

func bottomParameterSpecs() map[string]strategy.ParameterSpec {
	return map[string]strategy.ParameterSpec{
		bottomLowBandParam:       {Default: 15, Min: 0, Max: 100},
		bottomSingleVolumeParam:  {Default: 2, Min: 0.01, Max: 100},
		bottomSingleRallyParam:   {Default: 5, Min: 0, Max: 1_000},
		bottomGradualDaysParam:   {Default: 3, Min: 1, Max: 30, Integer: true},
		bottomGradualVolumeParam: {Default: 1.2, Min: 0.01, Max: 100},
		bottomGradualRallyParam:  {Default: 2, Min: 0, Max: 1_000},
		bottomSurgeGapParam:      {Default: 3, Min: 0, Max: 30, Integer: true},
		bottomPullbackPctParam:   {Default: 20, Min: 0, Max: 100},
		bottomPullbackBarsParam:  {Default: 15, Min: 1, Max: 365, Integer: true},
		bottomJMinParam:          {Default: -20, Min: -200, Max: 200},
		bottomJMaxParam:          {Default: 20, Min: -200, Max: 200},
	}
}

func resolvedParameters(specifications map[string]strategy.ParameterSpec, supplied map[string]float64) map[string]float64 {
	resolved := make(map[string]float64, len(specifications))
	for name, specification := range specifications {
		resolved[name] = specification.Default
	}
	for name, value := range supplied {
		if _, known := resolved[name]; known {
			resolved[name] = value
		}
	}
	return resolved
}

func percentageChange(current, previous float64) float64 {
	if previous == 0 {
		return 0
	}
	return (current - previous) / previous * 100
}
