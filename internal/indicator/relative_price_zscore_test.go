package indicator_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
)

func TestRelativePriceZScoreUsesPopulationStandardDeviation(t *testing.T) {
	got, err := indicator.RelativePriceZScore(
		[]float64{100, 100 * math.E, 100 * math.E * math.E},
		[]float64{100, 100, 100},
		3,
	)

	require.NoError(t, err)
	assert.False(t, got.Valid(0))
	assert.False(t, got.Valid(1))
	assert.InDelta(t, 1/math.Sqrt(2.0/3.0), mustValue(t, got, 2), 1e-12)
}

func TestRelativePriceZScoreIsInvariantToConstantRebasing(t *testing.T) {
	primary := []float64{100, 110, 130, 120}
	comparison := []float64{50, 52, 55, 57}
	rebasedPrimary := []float64{1_000, 1_100, 1_300, 1_200}
	rebasedComparison := []float64{150, 156, 165, 171}

	original, err := indicator.RelativePriceZScore(primary, comparison, 3)
	require.NoError(t, err)
	rebased, err := indicator.RelativePriceZScore(rebasedPrimary, rebasedComparison, 3)
	require.NoError(t, err)

	for index := 2; index < original.Len(); index++ {
		assert.InDelta(t, mustValue(t, original, index), mustValue(t, rebased, index), 1e-12)
	}
}

func TestRelativePriceZScoreMarksFlatRatioWindowsInvalid(t *testing.T) {
	got, err := indicator.RelativePriceZScore(
		[]float64{100, 110, 120, 130},
		[]float64{50, 55, 60, 65},
		3,
	)

	require.NoError(t, err)
	assert.Equal(t, 4, got.Len())
	for index := 0; index < got.Len(); index++ {
		assert.False(t, got.Valid(index))
	}
}

func TestRelativePriceZScoreAppliesNearZeroDeviationThreshold(t *testing.T) {
	belowThreshold, err := indicator.RelativePriceZScore(
		[]float64{1, math.Exp(1e-12)},
		[]float64{1, 1},
		2,
	)
	require.NoError(t, err)
	assert.False(t, belowThreshold.Valid(1))

	aboveThreshold, err := indicator.RelativePriceZScore(
		[]float64{1, math.Exp(4e-12)},
		[]float64{1, 1},
		2,
	)
	require.NoError(t, err)
	value := mustValue(t, aboveThreshold, 1)
	assert.False(t, math.IsNaN(value))
	assert.False(t, math.IsInf(value, 0))
}

func TestRelativePriceZScorePreservesWarmupWhenPeriodExceedsHistory(t *testing.T) {
	got, err := indicator.RelativePriceZScore(
		[]float64{100, 110},
		[]float64{90, 95},
		3,
	)

	require.NoError(t, err)
	assert.Equal(t, 2, got.Len())
	assert.False(t, got.Valid(0))
	assert.False(t, got.Valid(1))
}

func TestRelativePriceZScoreRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		primary    []float64
		comparison []float64
		period     int
	}{
		{name: "unequal lengths", primary: []float64{100}, comparison: []float64{100, 101}, period: 2},
		{name: "zero period", primary: []float64{100, 101}, comparison: []float64{100, 100}, period: 0},
		{name: "one period", primary: []float64{100, 101}, comparison: []float64{100, 100}, period: 1},
		{name: "zero primary", primary: []float64{0, 101}, comparison: []float64{100, 100}, period: 2},
		{name: "zero comparison", primary: []float64{100, 101}, comparison: []float64{100, 0}, period: 2},
		{name: "negative primary", primary: []float64{100, -1}, comparison: []float64{100, 100}, period: 2},
		{name: "negative comparison", primary: []float64{100, 101}, comparison: []float64{100, -1}, period: 2},
		{name: "primary nan", primary: []float64{100, math.NaN()}, comparison: []float64{100, 100}, period: 2},
		{name: "comparison nan", primary: []float64{100, 101}, comparison: []float64{100, math.NaN()}, period: 2},
		{name: "primary positive infinity", primary: []float64{100, math.Inf(1)}, comparison: []float64{100, 100}, period: 2},
		{name: "comparison positive infinity", primary: []float64{100, 101}, comparison: []float64{100, math.Inf(1)}, period: 2},
		{name: "primary negative infinity", primary: []float64{100, math.Inf(-1)}, comparison: []float64{100, 100}, period: 2},
		{name: "comparison negative infinity", primary: []float64{100, 101}, comparison: []float64{100, math.Inf(-1)}, period: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := indicator.RelativePriceZScore(test.primary, test.comparison, test.period)

			require.ErrorIs(t, err, indicator.ErrInvalidRelativePrices)
		})
	}
}

func TestRelativePriceZScoreDoesNotMutateInputs(t *testing.T) {
	primary := []float64{100, 110, 120}
	comparison := []float64{90, 95, 100}
	wantPrimary := append([]float64(nil), primary...)
	wantComparison := append([]float64(nil), comparison...)

	_, err := indicator.RelativePriceZScore(primary, comparison, 2)

	require.NoError(t, err)
	assert.Equal(t, wantPrimary, primary)
	assert.Equal(t, wantComparison, comparison)
}

func TestRelativePriceZScoreIsPrefixInvariant(t *testing.T) {
	primary := []float64{100, 103, 108, 106, 112, 118}
	comparison := []float64{90, 91, 93, 95, 96, 99}
	prefix, err := indicator.RelativePriceZScore(primary[:5], comparison[:5], 3)
	require.NoError(t, err)
	complete, err := indicator.RelativePriceZScore(primary, comparison, 3)
	require.NoError(t, err)

	for index := 0; index < prefix.Len(); index++ {
		prefixValue, prefixValid := prefix.At(index)
		completeValue, completeValid := complete.At(index)
		assert.Equal(t, prefixValid, completeValid)
		if prefixValid {
			assert.InDelta(t, prefixValue, completeValue, 1e-12)
		}
	}
}
