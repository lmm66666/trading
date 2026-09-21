package indicator

import (
	"context"
	"fmt"
	"sort"

	"trading/internal/market"
)

// Set maps a validated feature key to its deterministic output series.
type Set map[string]Series

func Build(dataset market.Dataset, factors []market.AdjustmentFactor, refs []Ref) (Set, error) {
	return BuildContext(context.Background(), dataset, factors, refs)
}

// BuildContext computes a feature set and observes cancellation between refs.
func BuildContext(ctx context.Context, dataset market.Dataset, factors []market.AdjustmentFactor, refs []Ref) (Set, error) {
	cache := make(Set)
	return buildWithComputerContext(ctx, dataset, factors, refs, func(dataset market.Dataset, factors []market.AdjustmentFactor, ref Ref) (Series, error) {
		return compute(dataset, factors, ref, cache)
	})
}

func buildWithComputer(dataset market.Dataset, factors []market.AdjustmentFactor, refs []Ref, computer func(market.Dataset, []market.AdjustmentFactor, Ref) (Series, error)) (Set, error) {
	return buildWithComputerContext(context.Background(), dataset, factors, refs, computer)
}

func buildWithComputerContext(ctx context.Context, dataset market.Dataset, factors []market.AdjustmentFactor, refs []Ref, computer func(market.Dataset, []market.AdjustmentFactor, Ref) (Series, error)) (Set, error) {
	unique := make([]Ref, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		key := ref.Key()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, ref)
	}

	result := make(Set, len(unique))
	for _, ref := range unique {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		series, err := computer(dataset, factors, ref)
		if err != nil {
			return nil, fmt.Errorf("compute %s: %w", ref.Key(), err)
		}
		result[ref.Key()] = series
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func compute(dataset market.Dataset, factors []market.AdjustmentFactor, ref Ref, cache Set) (Series, error) {
	if dataset.Timeframe() != ref.Timeframe {
		return Series{}, fmt.Errorf("indicator: dataset timeframe %s does not match reference %s", timeframeKey(dataset.Timeframe()), timeframeKey(ref.Timeframe))
	}
	switch ref.Kind {
	case OHLC:
		return priceSeries(dataset, factors, ref.PriceView, ref.Field)
	case STDKind:
		input, err := priceSeries(dataset, factors, ref.PriceView, ref.Field)
		return stddevSeries(input, ref.Period), err
	case SMAKind:
		input, err := priceSeries(dataset, factors, ref.PriceView, ref.Field)
		return smaSeries(input, ref.Period), err
	case EMAKind:
		input, err := priceSeries(dataset, factors, ref.PriceView, ref.Field)
		return emaSeries(input, ref.Period), err
	case VolumeMA:
		return smaSeries(volumeSeries(dataset), ref.Period), nil
	case MACDKind:
		input, err := priceSeries(dataset, factors, ref.PriceView, Close)
		if err != nil {
			return Series{}, err
		}
		dif, dea, histogram := macd(input, ref.Fast, ref.Slow, ref.Signal)
		switch ref.Field {
		case DIF:
			return dif, nil
		case DEA:
			return dea, nil
		default:
			return histogram, nil
		}
	case KDJKind:
		high, err := priceSeries(dataset, factors, ref.PriceView, High)
		if err != nil {
			return Series{}, err
		}
		low, err := priceSeries(dataset, factors, ref.PriceView, Low)
		if err != nil {
			return Series{}, err
		}
		close, err := priceSeries(dataset, factors, ref.PriceView, Close)
		if err != nil {
			return Series{}, err
		}
		k, d, j := kdj(high, low, close, ref.Period)
		switch ref.Field {
		case K:
			return k, nil
		case D:
			return d, nil
		default:
			return j, nil
		}
	case RETZKind:
		// Cache only within this Build: datasets/versions never share intermediates.
		sourceKey := fmt.Sprintf("returns/%s/%s", timeframeKey(ref.Timeframe), priceViewKey(ref.PriceView))
		returns, exists := cache[sourceKey]
		if !exists {
			input, err := priceSeries(dataset, factors, ref.PriceView, Close)
			if err != nil {
				return Series{}, err
			}
			returns = logReturnSeries(input)
			cache[sourceKey] = returns
		}
		window := ref.Period
		if ref.Field == Regime {
			window = ref.Regime
		}
		zKey := fmt.Sprintf("%s/p=%d", sourceKey, window)
		z, exists := cache[zKey]
		if !exists {
			z = returnZScoreSeries(returns, window)
			cache[zKey] = z
		}
		if ref.Field != Smooth {
			return z, nil
		}
		smoothKey := fmt.Sprintf("%s/sm=%d", zKey, ref.Smooth)
		smoothed, exists := cache[smoothKey]
		if !exists {
			smoothed = emaSeries(z, ref.Smooth)
			cache[smoothKey] = smoothed
		}
		return smoothed, nil
	default:
		return Series{}, ErrInvalidRef
	}
}

func priceSeries(dataset market.Dataset, factors []market.AdjustmentFactor, view market.PriceView, field Field) (Series, error) {
	result := invalidSeries(dataset.Len())
	if view == market.Raw {
		for index := 0; index < dataset.Len(); index++ {
			bar := dataset.Bar(index)
			result.values[index] = float64(priceField(bar, field)) / float64(market.ValueScale)
			result.valid[index] = true
		}
		return result, nil
	}

	// Dataset bars are already ordered. Sort one response-owned factor copy and
	// advance a cursor once instead of cloning/sorting factors for every Bar.
	knownFactors := append([]market.AdjustmentFactor(nil), factors...)
	sort.SliceStable(knownFactors, func(i, j int) bool {
		return knownFactors[i].EffectiveTime.Before(knownFactors[j].EffectiveTime)
	})
	factorIndex := -1
	for index := 0; index < dataset.Len(); index++ {
		bar := dataset.Bar(index)
		raw := priceField(bar, field)
		for factorIndex+1 < len(knownFactors) && !knownFactors[factorIndex+1].EffectiveTime.After(bar.CloseTime) {
			factorIndex++
		}
		if factorIndex < 0 || knownFactors[factorIndex].Numerator <= 0 || knownFactors[factorIndex].Denominator <= 0 {
			return Series{}, fmt.Errorf("indicator: missing valid adjustment factor at bar %d", index)
		}
		factor := knownFactors[factorIndex]
		result.values[index] = (float64(raw) / float64(market.ValueScale)) * float64(factor.Numerator) / float64(factor.Denominator)
		result.valid[index] = true
	}
	return result, nil
}

func priceField(bar market.Bar, field Field) market.Price {
	switch field {
	case Open:
		return bar.Open
	case High:
		return bar.High
	case Low:
		return bar.Low
	default:
		return bar.Close
	}
}

func volumeSeries(dataset market.Dataset) Series {
	result := invalidSeries(dataset.Len())
	for index := 0; index < dataset.Len(); index++ {
		bar := dataset.Bar(index)
		result.values[index] = float64(bar.Volume)
		result.valid[index] = true
	}
	return result
}
