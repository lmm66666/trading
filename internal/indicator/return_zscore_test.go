package indicator

import (
	"math"
	"testing"
	"trading/internal/market"

	"github.com/stretchr/testify/require"
)

func TestLogReturnSeriesComputesLogReturnsAndHandlesGaps(t *testing.T) {
	close := newSeries([]float64{100, 110, 99, 0, 110}, []bool{true, true, true, true, true})
	returns := logReturnSeries(close)
	require.False(t, returns.Valid(0))
	value, ok := returns.At(1)
	require.True(t, ok)
	require.InDelta(t, math.Log(1.1), value, 1e-12)
	value, ok = returns.At(2)
	require.True(t, ok)
	require.InDelta(t, math.Log(0.9), value, 1e-12)
	require.False(t, returns.Valid(3))
	require.False(t, returns.Valid(4))

	// 非正收盘价后重新播种，不跨缺口产出涨幅。
	close = newSeries([]float64{100, 0, 99, 110}, []bool{true, true, true, true})
	returns = logReturnSeries(close)
	require.False(t, returns.Valid(1))
	require.False(t, returns.Valid(2))
	value, ok = returns.At(3)
	require.True(t, ok)
	require.InDelta(t, math.Log(110.0/99.0), value, 1e-12)

	// 无效点同样中断并重置。
	close = newSeries([]float64{100, 110, 105, 115}, []bool{true, false, true, true})
	returns = logReturnSeries(close)
	require.False(t, returns.Valid(1))
	require.False(t, returns.Valid(2))
	value, ok = returns.At(3)
	require.True(t, ok)
	require.InDelta(t, math.Log(115.0/105.0), value, 1e-12)
}

func TestReturnZScoreSeriesUsesPopulationStandardDeviation(t *testing.T) {
	input := newSeries([]float64{1, 2, 3, 4, 4, 4}, []bool{true, true, true, true, true, true})
	output := returnZScoreSeries(input, 3)
	require.False(t, output.Valid(1))
	value, ok := output.At(2)
	require.True(t, ok)
	require.InDelta(t, 1.0/math.Sqrt(2.0/3.0), value, 1e-12)
	value, ok = output.At(3)
	require.True(t, ok)
	require.InDelta(t, 1.0/math.Sqrt(2.0/3.0), value, 1e-12)
	// 常量窗口标准差为零，按阈值记无效。
	require.False(t, output.Valid(5))

	// 窗口内含无效点时对应点无效。
	input.valid[2] = false
	require.False(t, returnZScoreSeries(input, 3).Valid(3))
	require.False(t, returnZScoreSeries(input, 3).Valid(4))

	// 窗口小于 2 不产出。
	require.False(t, returnZScoreSeries(input, 1).Valid(5))

	// 前缀不变性。
	full := returnZScoreSeries(newSeries([]float64{1, 2, 3, 4, 5, 6}, []bool{true, true, true, true, true, true}), 3)
	prefix := returnZScoreSeries(newSeries([]float64{1, 2, 3, 4}, []bool{true, true, true, true}), 3)
	for i := 0; i < 4; i++ {
		a, av := full.At(i)
		b, bv := prefix.At(i)
		require.Equal(t, av, bv)
		require.Equal(t, a, b)
	}
}

func TestReturnZScoreComponents(t *testing.T) {
	// 构造对数涨幅为 {1, 3, 1, 3} 的收盘序列。
	close := newSeries([]float64{1, math.E, math.Exp(4), math.Exp(5), math.Exp(8)}, []bool{true, true, true, true, true})
	returns := logReturnSeries(close)
	histogram := returnZScoreSeries(returns, 2)
	smoothed := emaSeries(histogram, 2)
	regime := returnZScoreSeries(returns, 4)

	// histogram：窗口 2，涨幅序列从索引 1 起有效。
	require.False(t, histogram.Valid(1))
	expected := []float64{1, -1, 1}
	for offset, want := range expected {
		value, ok := histogram.At(2 + offset)
		require.True(t, ok)
		require.InDelta(t, want, value, 1e-9)
	}

	// smooth：EMA(2) 自 histogram 首个有效点播种，alpha = 2/3。
	require.False(t, smoothed.Valid(1))
	value, ok := smoothed.At(2)
	require.True(t, ok)
	require.InDelta(t, 1.0, value, 1e-9)
	value, ok = smoothed.At(3)
	require.True(t, ok)
	require.InDelta(t, -1.0/3.0, value, 1e-9)
	value, ok = smoothed.At(4)
	require.True(t, ok)
	require.InDelta(t, 5.0/9.0, value, 1e-9)

	// regime：窗口 4，仅末点有效，涨幅 {1,3,1,3} 均值 2、总体标准差 1。
	require.False(t, regime.Valid(3))
	value, ok = regime.At(4)
	require.True(t, ok)
	require.InDelta(t, 1.0, value, 1e-9)
}

func TestLegacyRefsRejectRETZParameters(t *testing.T) {
	for _, ref := range []Ref{
		{Kind: OHLC, Timeframe: market.Day, PriceView: market.Raw, Field: Close},
		{Kind: SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close, Period: 5},
		{Kind: EMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close, Period: 5},
		{Kind: STDKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close, Period: 5},
		{Kind: VolumeMA, Timeframe: market.Day, PriceView: market.Raw, Field: Volume, Period: 5},
		{Kind: MACDKind, Timeframe: market.Day, PriceView: market.Raw, Field: DIF, Fast: 12, Slow: 26, Signal: 9},
		{Kind: KDJKind, Timeframe: market.Day, PriceView: market.Raw, Field: K, Period: 9},
	} {
		ref.Smooth = 5
		require.ErrorIs(t, ref.Validate(), ErrInvalidRef)
		require.Empty(t, ref.Key())
		ref.Smooth = 0
		ref.Regime = 252
		require.ErrorIs(t, ref.Validate(), ErrInvalidRef)
	}
}
