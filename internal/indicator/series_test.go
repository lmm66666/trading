package indicator_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
)

func TestSMAIsInvalidUntilFullPeriod(t *testing.T) {
	got := indicator.SMA([]float64{1, 2, 3, 4}, 3)

	assert.False(t, got.Valid(0))
	assert.False(t, got.Valid(1))
	assert.Equal(t, 2.0, mustValue(t, got, 2))
	assert.Equal(t, 3.0, mustValue(t, got, 3))
}

func TestSeriesSlicePreservesValuesAndValidity(t *testing.T) {
	got := indicator.SMA([]float64{2, 4, 6, 8}, 2).Slice(1, 4)

	require.Equal(t, 3, got.Len())
	assert.Equal(t, 3.0, mustValue(t, got, 0))
	assert.Equal(t, 5.0, mustValue(t, got, 1))
	assert.Equal(t, 7.0, mustValue(t, got, 2))
}

func mustValue(t *testing.T, series indicator.Series, index int) float64 {
	t.Helper()
	value, valid := series.At(index)
	require.True(t, valid, "expected index %d to be valid", index)
	return value
}
