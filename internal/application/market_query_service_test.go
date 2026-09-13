package application

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func TestPricesReadExactRequestedVersionAndView(t *testing.T) {
	data := &marketReadFake{latest: 8, stored: map[market.Timeframe][]market.Bar{market.Day: {marketBar(market.Day, 9)}}, factors: []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Numerator: 4, Denominator: 5}}}
	data.read = func(id market.InstrumentID, tf market.Timeframe, from, to time.Time, v market.DataVersion) {
		require.EqualValues(t, 7, v)
		require.Equal(t, marketDate(1), from)
		require.Equal(t, marketDate(31), to)
	}
	svc := NewMarketQueryService(data)
	got, err := svc.Prices(context.Background(), PriceQuery{Instrument: marketID, Timeframe: market.Day, Version: 7, View: market.ForwardAdjusted, From: marketDate(1), To: marketDate(31)})
	require.NoError(t, err)
	require.EqualValues(t, 7, got.DataVersion)
	require.Equal(t, 8.0, got.Bars[0].Close)
	require.Equal(t, time.UTC, got.Bars[0].CloseTime.Location())
}
func TestPricesDefaultsAndLimit(t *testing.T) {
	data := &marketReadFake{latest: 3, stored: map[market.Timeframe][]market.Bar{market.Day: {marketBar(market.Day, 8), marketBar(market.Day, 9)}}}
	got, err := NewMarketQueryService(data).Prices(context.Background(), PriceQuery{Instrument: marketID, Timeframe: market.Day, From: marketDate(1), To: marketDate(31), Limit: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, got.DataVersion)
	require.Len(t, got.Bars, 1)
	require.Equal(t, marketDate(9), got.Bars[0].CloseTime)
	require.Equal(t, 10.0, got.Bars[0].Close)
}
func TestPricesRejectsMissingFactorsAndInvalidInput(t *testing.T) {
	data := &marketReadFake{latest: 3, stored: map[market.Timeframe][]market.Bar{market.Day: {marketBar(market.Day, 9)}}}
	svc := NewMarketQueryService(data)
	good := PriceQuery{Instrument: marketID, Timeframe: market.Day, From: marketDate(1), To: marketDate(31)}
	for _, mutate := range []func(*PriceQuery){func(q *PriceQuery) { q.Limit = 5001 }, func(q *PriceQuery) { q.Limit = -1 }, func(q *PriceQuery) { q.Timeframe = market.Month }, func(q *PriceQuery) { q.View = 99 }, func(q *PriceQuery) { q.Instrument = market.InstrumentID{} }, func(q *PriceQuery) { q.To = q.From.Add(-time.Hour) }, func(q *PriceQuery) { q.From = q.From.In(time.FixedZone("local", 3600)) }, func(q *PriceQuery) { q.From = q.To.AddDate(-21, 0, 0) }} {
		q := good
		mutate(&q)
		_, err := svc.Prices(context.Background(), q)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
	good.View = market.ForwardAdjusted
	_, err := svc.Prices(context.Background(), good)
	require.ErrorIs(t, err, ErrIncompleteMarketData)
	data.latestErr = port.ErrTemporary
	_, err = svc.Prices(context.Background(), good)
	require.ErrorIs(t, err, port.ErrTemporary)
}
func TestPricesEmptyWindowStorageFailureAndCancellation(t *testing.T) {
	data := &marketReadFake{latest: 1}
	svc := NewMarketQueryService(data)
	q := PriceQuery{Instrument: marketID, Timeframe: market.Day}
	got, err := svc.Prices(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, got.Bars)
	data.errorRead = port.ErrTemporary
	_, err = svc.Prices(context.Background(), q)
	require.ErrorIs(t, err, port.ErrTemporary)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.Prices(ctx, q)
	require.ErrorIs(t, err, context.Canceled)
	_, err = NewMarketQueryService(nil).Prices(context.Background(), q)
	require.ErrorIs(t, err, ErrInvalidRequest)
	data.errorRead = nil
	data.factors = []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Numerator: 0, Denominator: 1}}
	q.View = market.ForwardAdjusted
	_, err = svc.Prices(context.Background(), q)
	require.ErrorIs(t, err, ErrIncompleteMarketData)
}
func TestPricesCapsDefaultAtFiveThousandLatestBars(t *testing.T) {
	data := &marketReadFake{latest: 1, stored: map[market.Timeframe][]market.Bar{}}
	for i := 0; i < 5100; i++ {
		data.stored[market.Day] = append(data.stored[market.Day], marketBar(market.Day, i+1))
	}
	got, err := NewMarketQueryService(data).Prices(context.Background(), PriceQuery{Instrument: marketID, Timeframe: market.Day, From: marketDate(1), To: marketDate(5100)})
	require.NoError(t, err)
	require.Len(t, got.Bars, 5000)
	require.Equal(t, marketDate(101), got.Bars[0].CloseTime)
}
