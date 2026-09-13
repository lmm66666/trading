package indicator

import (
	"fmt"

	"trading/internal/market"
)

// Set maps a validated feature key to its deterministic output series.
type Set map[string]Series

func Build(dataset market.Dataset, factors []market.AdjustmentFactor, refs []Ref) (Set, error) {
	return buildWithComputer(dataset, factors, refs, compute)
}

func buildWithComputer(dataset market.Dataset, factors []market.AdjustmentFactor, refs []Ref, computer func(market.Dataset, []market.AdjustmentFactor, Ref) (Series, error)) (Set, error) {
	unique := make([]Ref, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
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
		series, err := computer(dataset, factors, ref)
		if err != nil {
			return nil, fmt.Errorf("compute %s: %w", ref.Key(), err)
		}
		result[ref.Key()] = series
	}
	return result, nil
}

func compute(dataset market.Dataset, factors []market.AdjustmentFactor, ref Ref) (Series, error) {
	if dataset.Timeframe() != ref.Timeframe {
		return Series{}, fmt.Errorf("indicator: dataset timeframe %s does not match reference %s", timeframeKey(dataset.Timeframe()), timeframeKey(ref.Timeframe))
	}
	switch ref.Kind {
	case OHLC:
		return priceSeries(dataset, factors, ref.PriceView, ref.Field)
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
	default:
		return Series{}, ErrInvalidRef
	}
}

func priceSeries(dataset market.Dataset, factors []market.AdjustmentFactor, view market.PriceView, field Field) (Series, error) {
	result := invalidSeries(dataset.Len())
	for index, bar := range dataset.Bars() {
		raw := priceField(bar, field)
		if view == market.Raw {
			result.values[index] = float64(raw) / float64(market.ValueScale)
			result.valid[index] = true
			continue
		}
		adjusted, ok := market.AdjustedPrice(raw, bar.CloseTime, factors)
		if !ok {
			return Series{}, fmt.Errorf("indicator: missing valid adjustment factor at bar %d", index)
		}
		result.values[index] = adjusted
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
	for index, bar := range dataset.Bars() {
		result.values[index] = float64(bar.Volume)
		result.valid[index] = true
	}
	return result
}
