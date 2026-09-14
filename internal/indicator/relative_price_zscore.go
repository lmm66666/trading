package indicator

import (
	"errors"
	"fmt"
	"math"
)

var ErrInvalidRelativePrices = errors.New("indicator: invalid relative prices")

// RelativePriceZScore standardizes the rolling log price ratio of two
// caller-aligned price series using population standard deviation.
func RelativePriceZScore(primaryPrices, comparisonPrices []float64, period int) (Series, error) {
	if len(primaryPrices) != len(comparisonPrices) {
		return Series{}, fmt.Errorf("%w: price series lengths differ", ErrInvalidRelativePrices)
	}
	if period < 2 {
		return Series{}, fmt.Errorf("%w: period must be at least two", ErrInvalidRelativePrices)
	}

	spreads := make([]float64, len(primaryPrices))
	for index := range primaryPrices {
		if invalidRelativePrice(primaryPrices[index]) || invalidRelativePrice(comparisonPrices[index]) {
			return Series{}, fmt.Errorf("%w: prices must be positive and finite at index %d", ErrInvalidRelativePrices, index)
		}
		spreads[index] = math.Log(primaryPrices[index]) - math.Log(comparisonPrices[index])
	}

	result := invalidSeries(len(primaryPrices))
	for index := period - 1; index < len(spreads); index++ {
		start := index - period + 1
		mean := 0.0
		for _, spread := range spreads[start : index+1] {
			mean += spread
		}
		mean /= float64(period)

		variance := 0.0
		for _, spread := range spreads[start : index+1] {
			delta := spread - mean
			variance += delta * delta
		}
		standardDeviation := math.Sqrt(variance / float64(period))
		if standardDeviation <= 1e-12 {
			continue
		}
		result.values[index] = (spreads[index] - mean) / standardDeviation
		result.valid[index] = true
	}
	return result, nil
}

func invalidRelativePrice(price float64) bool {
	return price <= 0 || math.IsNaN(price) || math.IsInf(price, 0)
}
