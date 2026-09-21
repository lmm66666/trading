package indicator_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
	"trading/internal/market"
)

func TestEveryBuiltInFeatureIsPrefixInvariant(t *testing.T) {
	for _, ref := range builtInRefs() {
		all := mustBuild(t, testDataset(t, 160), ref)
		for _, size := range []int{20, 60, 120} {
			prefix := mustBuild(t, testDataset(t, size), ref)
			assertSeriesEqual(t, all.Slice(0, size), prefix)
		}
	}
}

func builtInRefs() []indicator.Ref {
	return []indicator.Ref{
		{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close},
		{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.High},
		{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close, Period: 20},
		{Kind: indicator.EMAKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close, Period: 12},
		{Kind: indicator.VolumeMA, Timeframe: market.Day, Field: indicator.Volume, Period: 20},
		{Kind: indicator.MACDKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.DIF, Fast: 12, Slow: 26, Signal: 9},
		{Kind: indicator.MACDKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Histogram, Fast: 12, Slow: 26, Signal: 9},
		{Kind: indicator.KDJKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.K, Period: 9},
		{Kind: indicator.KDJKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.J, Period: 9},
		{Kind: indicator.RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Histogram, Period: 10, Smooth: 3, Regime: 20},
		{Kind: indicator.RETZKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Smooth, Period: 10, Smooth: 3, Regime: 20},
		{Kind: indicator.RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Regime, Period: 10, Smooth: 3, Regime: 20},
	}
}

func mustBuild(t *testing.T, dataset market.Dataset, ref indicator.Ref) indicator.Series {
	t.Helper()
	series, err := indicator.Build(dataset, adjustmentFactors(), []indicator.Ref{ref})
	require.NoError(t, err)
	got, ok := series[ref.Key()]
	require.True(t, ok)
	return got
}

func assertSeriesEqual(t *testing.T, want, got indicator.Series) {
	t.Helper()
	require.Equal(t, want.Len(), got.Len())
	for index := range want.Len() {
		wantValue, wantValid := want.At(index)
		gotValue, gotValid := got.At(index)
		require.Equal(t, wantValid, gotValid, "index %d", index)
		if wantValid {
			require.InDelta(t, wantValue, gotValue, 1e-12, "index %d", index)
		}
	}
}

func testDataset(t *testing.T, length int) market.Dataset {
	t.Helper()
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	bars := make([]market.Bar, length)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range bars {
		close := market.Price(int64(100+index) * market.ValueScale)
		bars[index] = market.Bar{
			Instrument: id, Timeframe: market.Day,
			OpenTime: start.AddDate(0, 0, index), CloseTime: start.AddDate(0, 0, index).Add(8 * time.Hour),
			Open: close - 500, High: close + 1_000, Low: close - 1_000, Close: close,
			Volume: int64(1000 + index*10), Amount: market.Money(close * 100), Trading: market.Tradable, Version: 1,
		}
	}
	dataset, err := market.NewDataset(id, market.Day, 1, bars)
	require.NoError(t, err)
	return dataset
}

func adjustmentFactors() []market.AdjustmentFactor {
	return []market.AdjustmentFactor{{
		EffectiveTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		Numerator:     9, Denominator: 10, Version: 1,
	}}
}

func ExampleRef_Key() {
	ref := indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close, Period: 20}
	fmt.Println(ref.Key())
	// Output: sma/day/qfq/close/p=20
}
