package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
)

type marketIngestionFake struct {
	id  market.InstrumentID
	err error
}

func (f *marketIngestionFake) Refresh(_ context.Context, id market.InstrumentID) (application.RefreshResult, error) {
	f.id = id
	return application.RefreshResult{Instrument: id, Version: 9, Quality: "COMPLETE", DailyBars: 20, WeeklyBars: 4}, f.err
}

type marketTriggerFake struct {
	calls int
	err   error
}

func (f *marketTriggerFake) TriggerNow(int) error { f.calls++; return f.err }

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

func TestLegacyMarketEndpointsUseVersionedServices(t *testing.T) {
	f := newKernelFixture(t)
	ingestion := &marketIngestionFake{}
	trigger := &marketTriggerFake{}
	query := &marketQueryFake{}
	f.services.MarketIngestion = ingestion
	f.services.MarketTrigger = trigger
	f.services.MarketQueries = query
	f.services.MarketWorkers = 3
	f.router = NewRouter(nil, nil, nil, nil, nil, f.services)

	w := kernelRequest(t, f, http.MethodPost, "/api/stocks/historical", `{"code":"600000"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, market.InstrumentID{Exchange: market.SSE, Code: "600000"}, ingestion.id)

	w = kernelRequest(t, f, http.MethodPost, "/api/stocks/append", "")
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	require.Equal(t, 1, trigger.calls)

	w = kernelRequest(t, f, http.MethodGet, "/api/stocks/price?code=600000&cycle=daily&pagesize=10&pagenum=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, market.Raw, query.query.View)
	require.Equal(t, 10, query.query.Limit)
	require.Equal(t, f.services.Clock(), query.query.To)
}

func TestMarketCutoverValidatesBoundariesAndMapsBusy(t *testing.T) {
	f := newKernelFixture(t)
	trigger := &marketTriggerFake{err: application.ErrRefreshAlreadyRunning}
	f.services.MarketIngestion = &marketIngestionFake{}
	f.services.MarketTrigger = trigger
	f.services.MarketQueries = &marketQueryFake{}
	f.services.MarketWorkers = 3
	f.router = NewRouter(nil, nil, nil, nil, nil, f.services)

	w := kernelRequest(t, f, http.MethodPost, "/api/stocks/append", "")
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "MARKET_REFRESH_ALREADY_RUNNING")

	for _, path := range []string{
		"/api/stocks/price?code=600000&cycle=monthly",
		"/api/stocks/price?code=600000&pagesize=0",
		"/api/stocks/price?code=600000&pagesize=1001",
		"/api/stocks/price?code=600000&pagesize=1000&pagenum=6",
		"/api/stocks/price?code=600000&view=post",
		"/api/stocks/price?code=600000&version=-1",
	} {
		w = kernelRequest(t, f, http.MethodGet, path, "")
		require.Equal(t, http.StatusBadRequest, w.Code, path+" "+w.Body.String())
	}

	f.lookup.ids = []market.InstrumentID{{Exchange: market.SSE, Code: "600000"}, {Exchange: market.SZSE, Code: "600000"}}
	w = kernelRequest(t, f, http.MethodGet, "/api/stocks/price?code=600000", "")
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	w = kernelRequest(t, f, http.MethodPost, "/api/stocks/historical", `{"code":"600000"}`)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestMarketPriceCompatibilityPagingAndVersionedView(t *testing.T) {
	f := newKernelFixture(t)
	bars := make([]application.PriceBar, 25)
	for i := range bars {
		bars[i].Close = float64(i + 1)
	}
	query := &marketQueryFake{bars: bars}
	f.services.MarketQueries = query
	f.router = NewRouter(nil, nil, nil, nil, nil, f.services)

	w := kernelRequest(t, f, http.MethodGet, "/api/stocks/price?code=600000&cycle=weekly&pagesize=10&pagenum=2&view=qfq&version=7", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, market.Week, query.query.Timeframe)
	require.Equal(t, market.ForwardAdjusted, query.query.View)
	require.Equal(t, market.DataVersion(7), query.query.Version)
	require.Equal(t, 20, query.query.Limit)
	require.Contains(t, w.Body.String(), `"close":6`)
	require.Contains(t, w.Body.String(), `"close":15`)
	require.NotContains(t, w.Body.String(), `"close":16`)
}
