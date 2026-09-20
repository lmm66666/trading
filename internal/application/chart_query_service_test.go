package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/market"
	"trading/internal/port"
)

type chartCatalogFake struct {
	item port.InstrumentSummary
	err  error
}

func (f chartCatalogFake) Search(context.Context, port.InstrumentSearch) ([]port.InstrumentSummary, error) {
	return nil, f.err
}

func (f chartCatalogFake) Get(context.Context, market.InstrumentID) (port.InstrumentSummary, error) {
	return f.item, f.err
}

func TestChartQueryRejectsInvalidRequests(t *testing.T) {
	data := &marketReadFake{latest: 7}
	service, err := NewChartQueryService(data, chartCatalogFake{item: port.InstrumentSummary{ID: marketID, Active: true}})
	require.NoError(t, err)
	valid := ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.Raw, Limit: 100}

	mutations := []func(*ChartQuery){
		func(q *ChartQuery) { q.Instrument = market.InstrumentID{} },
		func(q *ChartQuery) { q.Timeframe = market.Month },
		func(q *ChartQuery) { q.View = 99 },
		func(q *ChartQuery) { q.Limit = 99 },
		func(q *ChartQuery) { q.Limit = 1001 },
		func(q *ChartQuery) { q.Indicators = []IndicatorRequest{{Kind: IndicatorKind("RSI"), Period: 14}} },
		func(q *ChartQuery) { q.Indicators = []IndicatorRequest{{Kind: IndicatorSMA, Period: 0}} },
		func(q *ChartQuery) {
			q.Indicators = []IndicatorRequest{{Kind: IndicatorMACD, Fast: 26, Slow: 12, Signal: 9}}
		},
	}
	for index, mutate := range mutations {
		query := valid
		mutate(&query)
		_, err := service.Query(context.Background(), query)
		assert.ErrorIs(t, err, ErrInvalidRequest, "mutation %d", index)
	}
	query := valid
	for index := 0; index < 17; index++ {
		query.Indicators = append(query.Indicators, IndicatorRequest{Kind: IndicatorSMA, Period: index + 1})
	}
	_, err = service.Query(context.Background(), query)
	assert.ErrorIs(t, err, ErrInvalidRequest)

	query = valid
	for period := 485; period <= 500; period++ {
		query.Indicators = append(query.Indicators, IndicatorRequest{Kind: IndicatorKDJ, Period: period})
	}
	_, err = service.Query(context.Background(), query)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestChartQueryComputesBeforeTrimmingAndPinsVersion(t *testing.T) {
	bars := make([]market.Bar, 105)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range bars {
		at := start.AddDate(0, 0, index)
		price := market.Price((index + 1) * int(market.ValueScale))
		bars[index] = market.Bar{Instrument: marketID, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: price, High: price + 1000, Low: price - 1000, Close: price, Volume: int64(index + 1), Amount: market.Money(price), Trading: market.Tradable}
	}
	data := &marketReadFake{latest: 7, stored: map[market.Timeframe][]market.Bar{market.Day: bars}}
	service, err := NewChartQueryService(data, chartCatalogFake{item: port.InstrumentSummary{ID: marketID, Name: "浦发银行", Active: true, LotSize: 100}})
	require.NoError(t, err)

	result, err := service.Query(context.Background(), ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.Raw, Limit: 100, Indicators: []IndicatorRequest{{Kind: IndicatorSMA, Period: 5}}})
	require.NoError(t, err)
	assert.EqualValues(t, 7, result.DataVersion)
	require.Len(t, result.Bars, 100)
	assert.Equal(t, start.AddDate(0, 0, 5), result.Bars[0].CloseTime)
	assert.True(t, result.HasMore)
	require.NotNil(t, result.NextBefore)
	assert.Equal(t, result.Bars[0].CloseTime, *result.NextBefore)
	require.Len(t, result.Series, 1)
	require.NotEmpty(t, result.Series[0].Points)
	assert.Equal(t, result.Bars[0].CloseTime, result.Series[0].Points[0].Time)
	assert.Equal(t, 4.0, result.Series[0].Points[0].Value)

	data.read = func(_ market.InstrumentID, _ market.Timeframe, _ time.Time, to time.Time, version market.DataVersion) {
		assert.Equal(t, result.Bars[0].CloseTime.Add(-time.Microsecond), to)
		assert.EqualValues(t, 7, version)
	}
	older, err := service.Query(context.Background(), ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.Raw, Before: result.Bars[0].CloseTime, Limit: 100, DataVersion: result.DataVersion})
	require.NoError(t, err)
	require.Len(t, older.Bars, 5)
	assert.Equal(t, start, older.Bars[0].CloseTime)
}

func TestChartQueryExpandsMultiComponentIndicators(t *testing.T) {
	bars := make([]market.Bar, 40)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range bars {
		at := start.AddDate(0, 0, index)
		price := market.Price((100 + index) * int(market.ValueScale))
		bars[index] = market.Bar{Instrument: marketID, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: price, High: price + 1000, Low: price - 1000, Close: price, Volume: 100, Amount: market.Money(price), Trading: market.Tradable}
	}
	data := &marketReadFake{latest: 3, stored: map[market.Timeframe][]market.Bar{market.Day: bars}}
	service, err := NewChartQueryService(data, chartCatalogFake{item: port.InstrumentSummary{ID: marketID, Active: true}})
	require.NoError(t, err)

	result, err := service.Query(context.Background(), ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.Raw, Limit: 100, Indicators: []IndicatorRequest{{Kind: IndicatorMACD, Fast: 12, Slow: 26, Signal: 9}, {Kind: IndicatorKDJ, Period: 9}}})
	require.NoError(t, err)
	components := make([]string, 0, len(result.Series))
	for _, series := range result.Series {
		components = append(components, fmt.Sprintf("%s:%s", series.Kind, series.Component))
	}
	assert.Equal(t, []string{"MACD:dif", "MACD:dea", "MACD:histogram", "KDJ:k", "KDJ:d", "KDJ:j"}, components)
}

func TestChartQueryAppliesForwardAdjustmentToBarsAndIndicators(t *testing.T) {
	bar := marketBar(market.Day, 2)
	data := &marketReadFake{latest: 4, stored: map[market.Timeframe][]market.Bar{market.Day: {bar}}, factors: []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Numerator: 1, Denominator: 2}}}
	service, err := NewChartQueryService(data, chartCatalogFake{item: port.InstrumentSummary{ID: marketID, Active: true}})
	require.NoError(t, err)

	result, err := service.Query(context.Background(), ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.ForwardAdjusted, Limit: 100, Indicators: []IndicatorRequest{{Kind: IndicatorEMA, Period: 5}}})
	require.NoError(t, err)
	require.Len(t, result.Bars, 1)
	assert.Equal(t, 5.0, result.Bars[0].Close)
	require.Len(t, result.Series, 1)
	require.Len(t, result.Series[0].Points, 1)
	assert.Equal(t, 5.0, result.Series[0].Points[0].Value)
}

func TestChartSTDComputesBeforeTrimmingAndPinsVersion(t *testing.T) {
	bars := make([]market.Bar, 105)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range bars {
		at := start.AddDate(0, 0, index)
		price := market.Price((index + 1) * int(market.ValueScale))
		bars[index] = market.Bar{Instrument: marketID, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: price, High: price + 1000, Low: price - 1000, Close: price, Volume: int64(index + 1), Amount: market.Money(price), Trading: market.Tradable}
	}
	data := &marketReadFake{latest: 7, stored: map[market.Timeframe][]market.Bar{market.Day: bars}}
	service, err := NewChartQueryService(data, chartCatalogFake{item: port.InstrumentSummary{ID: marketID, Name: "浦发银行", Active: true, LotSize: 100}})
	require.NoError(t, err)

	result, err := service.Query(context.Background(), ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.Raw, Limit: 100, Indicators: []IndicatorRequest{{Kind: IndicatorKind("STD"), Period: 5}}})
	require.NoError(t, err)
	assert.EqualValues(t, 7, result.DataVersion)
	require.Len(t, result.Bars, 100)
	assert.Equal(t, start.AddDate(0, 0, 5), result.Bars[0].CloseTime)
	assert.True(t, result.HasMore)
	require.NotNil(t, result.NextBefore)
	assert.Equal(t, result.Bars[0].CloseTime, *result.NextBefore)
	require.Len(t, result.Series, 1)
	require.NotEmpty(t, result.Series[0].Points)
	assert.Equal(t, result.Bars[0].CloseTime, result.Series[0].Points[0].Time)
	assert.InDelta(t, 1.4142135623730951, result.Series[0].Points[0].Value, 1e-10)

	data.read = func(_ market.InstrumentID, _ market.Timeframe, _ time.Time, to time.Time, version market.DataVersion) {
		assert.Equal(t, result.Bars[0].CloseTime.Add(-time.Microsecond), to)
		assert.EqualValues(t, 7, version)
	}
	older, err := service.Query(context.Background(), ChartQuery{Instrument: marketID, Timeframe: market.Day, View: market.Raw, Before: result.Bars[0].CloseTime, Limit: 100, DataVersion: result.DataVersion})
	require.NoError(t, err)
	require.Len(t, older.Bars, 5)
	assert.Equal(t, start, older.Bars[0].CloseTime)
}
