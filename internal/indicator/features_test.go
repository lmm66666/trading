package indicator_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
	"trading/internal/market"
)

func TestEMAUsesFirstValueAsRecursiveSeed(t *testing.T) {
	got := indicator.EMA([]float64{1, 2, 3}, 3)

	assert.Equal(t, 1.0, mustValue(t, got, 0))
	assert.Equal(t, 1.5, mustValue(t, got, 1))
	assert.Equal(t, 2.25, mustValue(t, got, 2))
}

func TestInvalidPeriodsProduceInvalidSeries(t *testing.T) {
	for _, series := range []indicator.Series{
		indicator.SMA([]float64{1, 2}, 0),
		indicator.EMA([]float64{1, 2}, -1),
	} {
		assert.Equal(t, 2, series.Len())
		assert.False(t, series.Valid(0))
		assert.False(t, series.Valid(1))
	}
}

func TestBuildExposesRawAndAdjustedOHLC(t *testing.T) {
	dataset := priceDataset(t)
	refs := []indicator.Ref{
		{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Open},
		{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.High},
	}
	got, err := indicator.Build(dataset, []market.AdjustmentFactor{{
		EffectiveTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Numerator: 1, Denominator: 2,
	}}, refs)

	require.NoError(t, err)
	assert.Equal(t, 10.0, mustValue(t, got[refs[0].Key()], 0))
	assert.Equal(t, 5.5, mustValue(t, got[refs[1].Key()], 0))
}

func TestBuildComputesVolumeMACDAndKDJ(t *testing.T) {
	dataset := priceDataset(t)
	refs := []indicator.Ref{
		{Kind: indicator.VolumeMA, Timeframe: market.Day, Field: indicator.Volume, Period: 2},
		{Kind: indicator.MACDKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Histogram, Fast: 2, Slow: 3, Signal: 2},
		{Kind: indicator.KDJKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.J, Period: 2},
	}
	got, err := indicator.Build(dataset, nil, refs)

	require.NoError(t, err)
	assert.False(t, got[refs[0].Key()].Valid(0))
	assert.Equal(t, 150.0, mustValue(t, got[refs[0].Key()], 1))
	assert.Equal(t, 250.0, mustValue(t, got[refs[0].Key()], 2))
	assert.Equal(t, 0.0, mustValue(t, got[refs[1].Key()], 0))
	assert.False(t, got[refs[2].Key()].Valid(0))
	assert.True(t, got[refs[2].Key()].Valid(1))
}

func TestBuildRejectsMissingFactorAndMismatchedTimeframe(t *testing.T) {
	dataset := priceDataset(t)
	adjusted := indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close}
	_, err := indicator.Build(dataset, nil, []indicator.Ref{adjusted})
	assert.Error(t, err)

	wrongTimeframe := indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close}
	_, err = indicator.Build(dataset, nil, []indicator.Ref{wrongTimeframe})
	assert.Error(t, err)
}

func TestBuildAdjustedSeriesAllocationsAreBoundedPerSeries(t *testing.T) {
	dataset := testDataset(t, 140)
	ref := indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: indicator.Close}
	factors := adjustmentFactors()
	var buildErr error
	allocations := testing.AllocsPerRun(10, func() {
		_, buildErr = indicator.Build(dataset, factors, []indicator.Ref{ref})
	})
	require.NoError(t, buildErr)
	require.Less(t, allocations, 50.0, "复权因子不能按每根 Bar 重复复制和排序")
}

func TestSeriesRejectsInvalidIndexesAndRanges(t *testing.T) {
	series := indicator.SMA([]float64{1, 2, 3}, 2)
	_, valid := series.At(-1)
	assert.False(t, valid)
	_, valid = series.At(3)
	assert.False(t, valid)
	assert.Zero(t, series.Slice(-1, 1).Len())
	assert.Zero(t, series.Slice(2, 1).Len())
	assert.Zero(t, series.Slice(0, 4).Len())
}

func priceDataset(t *testing.T) market.Dataset {
	t.Helper()
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := []market.Bar{
		{Instrument: id, Timeframe: market.Day, OpenTime: start, CloseTime: start.Add(8 * time.Hour), Open: 100_000, High: 110_000, Low: 90_000, Close: 100_000, Volume: 100, Amount: 100_000, Trading: market.Tradable, Version: 1},
		{Instrument: id, Timeframe: market.Day, OpenTime: start.AddDate(0, 0, 1), CloseTime: start.AddDate(0, 0, 1).Add(8 * time.Hour), Open: 110_000, High: 120_000, Low: 100_000, Close: 110_000, Volume: 200, Amount: 220_000, Trading: market.Tradable, Version: 1},
		{Instrument: id, Timeframe: market.Day, OpenTime: start.AddDate(0, 0, 2), CloseTime: start.AddDate(0, 0, 2).Add(8 * time.Hour), Open: 120_000, High: 130_000, Low: 110_000, Close: 120_000, Volume: 300, Amount: 360_000, Trading: market.Tradable, Version: 1},
	}
	dataset, err := market.NewDataset(id, market.Day, 1, bars)
	require.NoError(t, err)
	return dataset
}
