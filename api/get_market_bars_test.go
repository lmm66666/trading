package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/application"
	"trading/internal/market"
)

type marketQueryFake struct {
	query application.PriceQuery
	bars  []application.PriceBar
	err   error
}

func (f *marketQueryFake) Prices(_ context.Context, q application.PriceQuery) (application.PriceResult, error) {
	f.query = q
	bars := f.bars
	if bars == nil {
		bars = []application.PriceBar{{CloseTime: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Close: 10.5}}
	}
	return application.PriceResult{Instrument: q.Instrument, Timeframe: q.Timeframe, View: q.View, DataVersion: 9, Bars: bars}, f.err
}

func TestGetMarketBarsQueriesFuturesByCanonicalInstrument(t *testing.T) {
	query := &marketQueryFake{bars: []application.PriceBar{{CloseTime: time.Date(2026, 9, 11, 7, 0, 0, 0, time.UTC), Close: 944.94}}}
	router := NewRouter(KernelServices{MarketQueries: query, Clock: func() time.Time {
		return time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	}})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/market/bars?instrument=SHFE%3AAU.MAIN&timeframe=daily&limit=20&version=7", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}, query.query.Instrument)
	require.Equal(t, market.Day, query.query.Timeframe)
	require.Equal(t, market.Raw, query.query.View)
	require.Equal(t, market.DataVersion(7), query.query.Version)
	require.Equal(t, 20, query.query.Limit)
	require.Contains(t, response.Body.String(), `"instrument":"SHFE:AU.MAIN"`)
	require.Contains(t, response.Body.String(), `"close":944.94`)
}

func TestGetMarketBarsRejectsMissingDuplicateAndUnknownQueries(t *testing.T) {
	router := NewRouter(KernelServices{MarketQueries: &marketQueryFake{}})
	for _, target := range []string{
		"/api/v1/market/bars",
		"/api/v1/market/bars?instrument=SHFE%3AAU.MAIN&instrument=INE%3ASC.MAIN",
		"/api/v1/market/bars?instrument=SHFE%3AAU.MAIN&url=https://attacker.invalid",
		"/api/v1/market/bars?instrument=SHFE%3AAU.MAIN&timeframe=minute",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusBadRequest, response.Code, target)
	}
}
