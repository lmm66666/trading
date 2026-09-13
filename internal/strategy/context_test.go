package strategy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
	"trading/internal/market"
)

var closeRef = indicator.Ref{
	Kind: indicator.OHLC, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close,
}

func TestContextRejectsFutureAccess(t *testing.T) {
	ctx := newTestContext(t, 2)

	_, ok := ctx.Float(closeRef, -1)

	assert.False(t, ok)
	assert.ErrorIs(t, ctx.Err(), ErrFutureAccess)
}

func TestContextExposesOnlyCurrentAndHistoricalFeatureValues(t *testing.T) {
	ctx := newTestContext(t, 2)

	current, currentOK := ctx.Float(closeRef, 0)
	previous, previousOK := ctx.Float(closeRef, 1)
	_, unavailable := ctx.Float(closeRef, 3)
	_, unavailableAuxiliary := ctx.Float(indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close}, 0)

	assert.Equal(t, 12.0, current)
	assert.True(t, currentOK)
	assert.Equal(t, 11.0, previous)
	assert.True(t, previousOK)
	assert.False(t, unavailable)
	assert.False(t, unavailableAuxiliary)
	assert.NoError(t, ctx.Err())
	assert.Equal(t, 2, ctx.Index())
	assert.Equal(t, market.Price(120_000), ctx.Bar().Close)
	assert.Equal(t, PositionView{}, ctx.Position())
}

func TestNewTimelineRejectsInvalidPrimaryFeatureSet(t *testing.T) {
	primary := testDataset(t, market.Day, []string{"2026-01-01", "2026-01-02", "2026-01-03"})

	_, err := NewTimeline(primary, indicator.Set{"": indicator.SMA([]float64{1, 2, 3}, 1)}, nil)
	assert.ErrorIs(t, err, ErrInvalidTimeline)
	_, err = NewTimeline(primary, indicator.Set{"not-an-indicator-key": indicator.SMA([]float64{1, 2, 3}, 1)}, nil)
	assert.ErrorIs(t, err, ErrInvalidTimeline)

	_, err = NewTimeline(primary, indicator.Set{closeRef.Key(): indicator.SMA([]float64{1, 2}, 1)}, nil)
	assert.ErrorIs(t, err, ErrInvalidTimeline)
}

func TestNewTimelineRejectsFutureOrOutOfRangeAuxiliaryAlignment(t *testing.T) {
	primary := testDataset(t, market.Day, []string{"2026-01-01", "2026-01-02", "2026-01-03"})
	auxiliary := testDataset(t, market.Week, []string{"2026-01-01", "2026-01-03"})
	auxRef := indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close}
	auxFeatures := indicator.Set{auxRef.Key(): indicator.SMA([]float64{20, 21}, 1)}

	_, err := NewTimeline(primary, indicator.Set{closeRef.Key(): indicator.SMA([]float64{10, 11, 12}, 1)}, map[market.Timeframe]AlignedFeatures{
		market.Week: {Dataset: auxiliary, Features: auxFeatures, PrimaryToAuxiliary: []int{1, 1, 1}},
	})
	assert.ErrorIs(t, err, ErrInvalidTimeline)

	_, err = NewTimeline(primary, indicator.Set{closeRef.Key(): indicator.SMA([]float64{10, 11, 12}, 1)}, map[market.Timeframe]AlignedFeatures{
		market.Week: {Dataset: auxiliary, Features: auxFeatures, PrimaryToAuxiliary: []int{0, 0, 2}},
	})
	assert.ErrorIs(t, err, ErrInvalidTimeline)
}

func TestContextReadsAuxiliaryFeaturesAsOfTheRequestedPrimaryBar(t *testing.T) {
	primary := testDataset(t, market.Day, []string{"2026-01-01", "2026-01-02", "2026-01-03"})
	auxiliary := testDataset(t, market.Week, []string{"2026-01-01", "2026-01-03"})
	auxRef := indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close}
	timeline, err := NewTimeline(primary, indicator.Set{closeRef.Key(): indicator.SMA([]float64{10, 11, 12}, 1)}, map[market.Timeframe]AlignedFeatures{
		market.Week: {
			Dataset: auxiliary, Features: indicator.Set{auxRef.Key(): indicator.SMA([]float64{20, 21}, 1)},
			PrimaryToAuxiliary: []int{0, 0, 1},
		},
	})
	require.NoError(t, err)

	ctx := newContext(timeline, 2, PositionView{Open: true, Quantity: 100, HoldingBars: 2})
	current, currentOK := ctx.Float(auxRef, 0)
	previous, previousOK := ctx.Float(auxRef, 1)
	_, unknown := ctx.Float(indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close, Period: 2}, 0)

	assert.Equal(t, 21.0, current)
	assert.True(t, currentOK)
	assert.Equal(t, 20.0, previous)
	assert.True(t, previousOK)
	assert.False(t, unknown)
	assert.Equal(t, PositionView{Open: true, Quantity: 100, HoldingBars: 2}, ctx.Position())
}

func TestNewTimelineDefensivelyCopiesFeatureMapsAndAlignment(t *testing.T) {
	primary := testDataset(t, market.Day, []string{"2026-01-01", "2026-01-02", "2026-01-03"})
	features := indicator.Set{closeRef.Key(): indicator.SMA([]float64{10, 11, 12}, 1)}
	timeline, err := NewTimeline(primary, features, nil)
	require.NoError(t, err)

	features[closeRef.Key()] = indicator.SMA([]float64{90, 91, 92}, 1)
	value, ok := newContext(timeline, 0, PositionView{}).Float(closeRef, 0)

	assert.True(t, ok)
	assert.Equal(t, 10.0, value)
}

func TestTimelineFeatureKeysMustRoundTripThroughIndicatorReferences(t *testing.T) {
	for _, key := range []string{
		"ohlc/day/raw/close",
		"ohlc/week/qfq/open",
		"sma/month/raw/close/p=2",
		"ema/day/raw/high/p=2",
		"volume_ma/day/raw/volume/p=2",
		"macd/day/raw/histogram/f=2/s=3/sig=2",
		"kdj/day/raw/j/p=2",
	} {
		assert.True(t, validFeatureKey(key), key)
	}
	for _, key := range []string{
		"ohlc/day/raw/close/extra",
		"ohlc/day/raw/nope",
		"sma/day/raw/close",
		"sma/day/raw/close/p=x",
		"volume_ma/day/qfq/volume/p=2",
		"macd/day/raw/histogram/f=2/s=3",
		"macd/day/raw/histogram/f=x/s=3/sig=2",
		"macd/day/raw/histogram/f=3/s=2/sig=2",
		"unknown/day/raw/close",
		"ohlc/year/raw/close",
		"ohlc/day/other/close",
	} {
		assert.False(t, validFeatureKey(key), key)
	}
}

func newTestContext(t *testing.T, index int) Context {
	t.Helper()
	primary := testDataset(t, market.Day, []string{"2026-01-01", "2026-01-02", "2026-01-03"})
	timeline, err := NewTimeline(primary, indicator.Set{closeRef.Key(): indicator.SMA([]float64{10, 11, 12}, 1)}, nil)
	require.NoError(t, err)
	return newContext(timeline, index, PositionView{})
}

func testDataset(t *testing.T, timeframe market.Timeframe, days []string) market.Dataset {
	t.Helper()
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	bars := make([]market.Bar, len(days))
	for i, day := range days {
		closeTime, err := time.Parse(time.DateOnly, day)
		require.NoError(t, err)
		close := market.Price((10 + i) * 10_000)
		bars[i] = market.Bar{
			Instrument: id, Timeframe: timeframe, OpenTime: closeTime.Add(-8 * time.Hour), CloseTime: closeTime,
			Open: close, High: close + 1, Low: close - 1, Close: close,
			Volume: 1, Amount: market.Money(close), Trading: market.Tradable, Version: 1,
		}
	}
	dataset, err := market.NewDataset(id, timeframe, 1, bars)
	require.NoError(t, err)
	return dataset
}
