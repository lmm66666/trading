package indicator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/market"
)

func TestBuildContextHonorsCancellationBeforeComputing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := BuildContext(ctx, market.Dataset{}, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestBuildComputesDuplicateReferenceOnce(t *testing.T) {
	ref := Ref{Kind: SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close, Period: 3}
	calls := 0
	computed, err := buildWithComputer(graphDataset(t, 5), nil, []Ref{ref, ref}, func(ds market.Dataset, factors []market.AdjustmentFactor, got Ref) (Series, error) {
		calls++
		return SMA([]float64{1, 2, 3, 4, 5}, 3), nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Len(t, computed, 1)
}

func TestBuildRejectsInvalidReferenceBeforeComputing(t *testing.T) {
	invalid := Ref{Kind: SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close}
	calls := 0
	_, err := buildWithComputer(graphDataset(t, 5), nil, []Ref{invalid}, func(market.Dataset, []market.AdjustmentFactor, Ref) (Series, error) {
		calls++
		return Series{}, nil
	})

	assert.ErrorIs(t, err, ErrInvalidRef)
	assert.Zero(t, calls)
}

func TestRefValidationAndKeyAreStable(t *testing.T) {
	valid := Ref{Kind: MACDKind, Timeframe: market.Week, PriceView: market.ForwardAdjusted, Field: DEA, Fast: 12, Slow: 26, Signal: 9}
	require.NoError(t, valid.Validate())
	assert.Equal(t, "macd/week/qfq/dea/f=12/s=26/sig=9", valid.Key())

	retz := Ref{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Histogram, Period: 126, Smooth: 5, Regime: 252}
	require.NoError(t, retz.Validate())
	assert.Equal(t, "retz/day/raw/histogram/p=126/sm=5/rg=252", retz.Key())
	assert.NotEqual(t, retz.Key(), Ref{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Regime, Period: 126, Smooth: 5, Regime: 252}.Key())

	invalid := []Ref{
		{Kind: SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close},
		{Kind: EMAKind, Timeframe: market.UnknownTimeframe, PriceView: market.Raw, Field: Close, Period: 3},
		{Kind: VolumeMA, Timeframe: market.Day, PriceView: market.ForwardAdjusted, Field: Volume, Period: 3},
		{Kind: MACDKind, Timeframe: market.Day, PriceView: market.Raw, Field: DIF, Fast: 26, Slow: 12, Signal: 9},
		{Kind: KDJKind, Timeframe: market.Day, PriceView: market.Raw, Field: J},
		{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Histogram, Period: 1, Smooth: 5, Regime: 252},
		{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Histogram, Period: 126, Smooth: 0, Regime: 252},
		{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Histogram, Period: 126, Smooth: 5, Regime: 126},
		{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Close, Period: 126, Smooth: 5, Regime: 252},
		{Kind: RETZKind, Timeframe: market.Day, PriceView: market.Raw, Field: Histogram, Period: 126, Smooth: 5, Regime: 252, Fast: 12},
	}
	for _, ref := range invalid {
		assert.ErrorIs(t, ref.Validate(), ErrInvalidRef)
	}

	assert.NotEqual(t, valid.Key(), Ref{Kind: MACDKind, Timeframe: market.Week, PriceView: market.Raw, Field: DEA, Fast: 12, Slow: 26, Signal: 9}.Key())
}

func TestBuildWrapsComputeFailures(t *testing.T) {
	ref := Ref{Kind: OHLC, Timeframe: market.Day, PriceView: market.Raw, Field: Close}
	expected := errors.New("factor unavailable")
	_, err := buildWithComputer(graphDataset(t, 2), nil, []Ref{ref}, func(market.Dataset, []market.AdjustmentFactor, Ref) (Series, error) {
		return Series{}, expected
	})
	assert.ErrorIs(t, err, expected)
}

func graphDataset(t *testing.T, length int) market.Dataset {
	t.Helper()
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	bars := make([]market.Bar, length)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range bars {
		close := market.Price(int64(100+index) * market.ValueScale)
		bars[index] = market.Bar{
			Instrument: id, Timeframe: market.Day, OpenTime: start.AddDate(0, 0, index), CloseTime: start.AddDate(0, 0, index).Add(8 * time.Hour),
			Open: close - 500, High: close + 1_000, Low: close - 1_000, Close: close, Volume: 100, Amount: market.Money(close * 100), Trading: market.Tradable, Version: 1,
		}
	}
	dataset, err := market.NewDataset(id, market.Day, 1, bars)
	require.NoError(t, err)
	return dataset
}
