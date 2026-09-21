package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

type apiChartQueries struct {
	query  application.ChartQuery
	result application.ChartResult
	err    error
}

func (s *apiChartQueries) Query(_ context.Context, query application.ChartQuery) (application.ChartResult, error) {
	s.query = query
	return s.result, s.err
}

func TestQueryChartMapsRequestAndResponse(t *testing.T) {
	f := newKernelFixture(t)
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	next := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	queries := &apiChartQueries{result: application.ChartResult{
		Instrument:  port.InstrumentSummary{ID: market.InstrumentID{Exchange: market.SZSE, Code: "002415"}, Name: "海康威视", Board: "MAIN", Active: true, LotSize: 100},
		Timeframe:   market.Day,
		View:        market.ForwardAdjusted,
		DataVersion: 8,
		Bars:        []application.PriceBar{{OpenTime: at, CloseTime: at, Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 100, Amount: 1050}},
		Series:      []application.ChartSeries{{Key: "sma/day/qfq/close/p=5", Kind: application.IndicatorSMA, Component: "value", Points: []application.ChartPoint{{Time: at, Value: 10.2}}}},
		HasMore:     true,
		NextBefore:  &next,
	}}
	f.services.ChartQueries = queries
	f.router = NewRouter(f.services)

	body := `{"instrument":"SZSE:002415","timeframe":"DAY","price_view":"QFQ","before":"2026-02-01T08:00:00+08:00","limit":400,"data_version":8,"indicators":[{"kind":"SMA","period":5}]}`
	w := kernelRequest(t, f, "POST", "/api/v1/chart-queries", body)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, market.InstrumentID{Exchange: market.SZSE, Code: "002415"}, queries.query.Instrument)
	assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), queries.query.Before)
	assert.EqualValues(t, 8, queries.query.DataVersion)
	require.Contains(t, w.Body.String(), `"instrument":"SZSE:002415"`)
	require.Contains(t, w.Body.String(), `"timeframe":"DAY"`)
	require.Contains(t, w.Body.String(), `"price_view":"QFQ"`)
	require.Contains(t, w.Body.String(), `"next_before":"2025-01-02T00:00:00Z"`)
	require.Contains(t, w.Body.String(), `"component":"value"`)
}

func TestQueryChartRejectsInvalidAndNonStrictBodies(t *testing.T) {
	for _, body := range []string{
		`{"instrument":"600000","timeframe":"DAY","price_view":"RAW"}`,
		`{"instrument":"SSE:600000","timeframe":"MONTH","price_view":"RAW"}`,
		`{"instrument":"SSE:600000","timeframe":"DAY","price_view":"HFQ"}`,
		`{"instrument":"SSE:600000","timeframe":"DAY","price_view":"RAW","extra":true}`,
		`{"instrument":"SSE:600000","timeframe":"DAY","price_view":"RAW"}{}`,
	} {
		f := newKernelFixture(t)
		f.services.ChartQueries = &apiChartQueries{}
		f.router = NewRouter(f.services)
		w := kernelRequest(t, f, "POST", "/api/v1/chart-queries", body)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "INVALID_REQUEST")
	}
}

func TestQueryChartZSCORETransport(t *testing.T) {
	f := newKernelFixture(t)
	queries := &apiChartQueries{result: application.ChartResult{Timeframe: market.Day, View: market.Raw, DataVersion: 7}}
	for _, component := range []string{"histogram", "smooth", "regime"} {
		queries.result.Series = append(queries.result.Series, application.ChartSeries{Kind: application.IndicatorZSCORE, Component: component})
	}
	queries.result.ZScores = []application.ZScoreDiagnostic{{Comparison: "SHFE:AU.MAIN", Warning: "关联期货暂无历史数据"}}
	f.services.ChartQueries = queries
	f.router = NewRouter(f.services)
	body := `{"instrument":"SSE:600000","comparison":"SHFE:AU.MAIN","timeframe":"DAY","price_view":"RAW","indicators":[{"kind":"ZSCORE","period":126,"smooth":5,"regime":252,"lag":1}]}`
	w := kernelRequest(t, f, "POST", "/api/v1/chart-queries", body)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, []application.IndicatorRequest{{Kind: application.IndicatorZSCORE, Period: 126, Smooth: 5, Regime: 252, Lag: 1}}, queries.query.Indicators)
	for _, component := range []string{"histogram", "smooth", "regime"} {
		require.Contains(t, w.Body.String(), `"component":"`+component+`"`)
	}
	require.Equal(t, "SHFE:AU.MAIN", queries.query.Comparison)
	require.Contains(t, w.Body.String(), `"zscores":[`)
	require.Contains(t, w.Body.String(), `"z":null`)
	queries.err = application.ErrInvalidRequest
	require.Equal(t, 400, kernelRequest(t, f, "POST", "/api/v1/chart-queries", body).Code)
}

func TestQueryChartAllowsUnselectedComparison(t *testing.T) {
	// Omitted, empty and null comparison all represent an unselected board.
	// Real application behavior is covered by TestZScoreFailureIsolation/missing.
	for _, comparison := range []string{"", `,"comparison":""`, `,"comparison":null`} {
		f := newKernelFixture(t)
		queries := &apiChartQueries{result: application.ChartResult{Timeframe: market.Day, View: market.Raw, ZScores: []application.ZScoreDiagnostic{{Warning: "请选择看板关联期货后计算 Z-score"}}}}
		f.services.ChartQueries = queries
		f.router = NewRouter(f.services)
		body := `{"instrument":"SZSE:300750","timeframe":"DAY","price_view":"RAW","indicators":[{"kind":"ZSCORE","period":126,"smooth":5,"regime":252}]` + comparison + `}`
		w := kernelRequest(t, f, "POST", "/api/v1/chart-queries", body)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Empty(t, queries.query.Comparison)
		require.Contains(t, w.Body.String(), "请选择看板关联期货")
	}
}
