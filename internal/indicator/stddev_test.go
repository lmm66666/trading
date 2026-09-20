package indicator

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"trading/internal/market"
)

func TestStandardDeviationPopulationWarmupAndPrefix(t *testing.T) {
	ref := Ref{Kind: Kind("std"), Timeframe: market.Day, PriceView: market.Raw, Field: Close, Period: 3}
	require.NoError(t, ref.Validate())
	require.Equal(t, "std/day/raw/close/p=3", ref.Key())
	input := newSeries([]float64{1, 2, 3, 4, 4, 4}, []bool{true, true, true, true, true, true})
	output := stddevSeries(input, 3)
	require.False(t, output.Valid(1))
	value, ok := output.At(2)
	require.True(t, ok)
	require.InDelta(t, math.Sqrt(2.0/3), value, 1e-12)
	value, ok = output.At(5)
	require.True(t, ok)
	require.Zero(t, value)
	prefix := stddevSeries(input.Slice(0, 4), 3)
	for i := 0; i < 4; i++ {
		a, av := output.At(i)
		b, bv := prefix.At(i)
		require.Equal(t, av, bv)
		require.Equal(t, a, b)
	}
	input.valid[2] = false
	require.False(t, stddevSeries(input, 3).Valid(3))
}
