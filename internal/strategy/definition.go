package strategy

import (
	"math"

	"trading/internal/indicator"
	"trading/internal/market"
)

// Definition is the static contract a compiled strategy exposes to the runner.
type Definition struct {
	ID               string
	Version          string
	PrimaryTimeframe market.Timeframe
	WarmupBars       int
	Features         []indicator.Ref
	Auxiliary        []market.Timeframe
	DefaultHoldBars  int
	Parameters       map[string]ParameterSpec
}

// ParameterSpec defines one numeric strategy parameter.
type ParameterSpec struct {
	Default float64
	Min     float64
	Max     float64
	Integer bool
}

func cloneDefinition(definition Definition) Definition {
	cloned := definition
	if definition.Features != nil {
		cloned.Features = make([]indicator.Ref, len(definition.Features))
		copy(cloned.Features, definition.Features)
	}
	if definition.Auxiliary != nil {
		cloned.Auxiliary = make([]market.Timeframe, len(definition.Auxiliary))
		copy(cloned.Auxiliary, definition.Auxiliary)
	}
	if definition.Parameters != nil {
		cloned.Parameters = make(map[string]ParameterSpec, len(definition.Parameters))
		for name, spec := range definition.Parameters {
			cloned.Parameters[name] = spec
		}
	}
	return cloned
}

func validDefinition(definition Definition) bool {
	if definition.ID == "" || definition.Version == "" || !definition.PrimaryTimeframe.Valid() ||
		definition.WarmupBars < 0 || definition.DefaultHoldBars < 0 {
		return false
	}
	auxiliary := make(map[market.Timeframe]struct{}, len(definition.Auxiliary))
	for _, timeframe := range definition.Auxiliary {
		if !timeframe.Valid() || timeframe == definition.PrimaryTimeframe {
			return false
		}
		if _, duplicate := auxiliary[timeframe]; duplicate {
			return false
		}
		auxiliary[timeframe] = struct{}{}
	}
	features := make(map[string]struct{}, len(definition.Features))
	for _, ref := range definition.Features {
		if ref.Validate() != nil || (ref.Timeframe != definition.PrimaryTimeframe && !containsTimeframe(auxiliary, ref.Timeframe)) {
			return false
		}
		key := ref.Key()
		if _, duplicate := features[key]; duplicate {
			return false
		}
		features[key] = struct{}{}
	}
	for name, spec := range definition.Parameters {
		if name == "" || !validParameter(spec, spec.Default) {
			return false
		}
	}
	return true
}

func containsTimeframe(timeframes map[market.Timeframe]struct{}, timeframe market.Timeframe) bool {
	_, exists := timeframes[timeframe]
	return exists
}

func validParameter(spec ParameterSpec, value float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.IsNaN(spec.Min) || math.IsNaN(spec.Max) ||
		math.IsInf(spec.Min, 0) || math.IsInf(spec.Max, 0) || spec.Min > spec.Max || value < spec.Min || value > spec.Max {
		return false
	}
	return !spec.Integer || math.Trunc(value) == value
}
