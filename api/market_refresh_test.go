package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

type refreshIngestionFake struct {
	id  market.InstrumentID
	err error
}

func (f *refreshIngestionFake) Refresh(_ context.Context, id market.InstrumentID) (application.RefreshResult, error) {
	f.id = id
	if f.err != nil {
		return application.RefreshResult{}, f.err
	}
	return application.RefreshResult{Instrument: id, Version: 5, Quality: "COMPLETE", DailyBars: 10, WeeklyBars: 2}, nil
}

type refreshTriggerFake struct {
	workers int
	err     error
}

func (f *refreshTriggerFake) TriggerNow(workers int) (port.RefreshReceipt, error) {
	f.workers = workers
	return port.RefreshReceipt{Status: "ACCEPTED", RunID: "run-1", ProgressAvailable: true}, f.err
}

type refreshLookupFake struct {
	ids []market.InstrumentID
	err error
}

func (f *refreshLookupFake) ResolveCode(_ context.Context, _ string) ([]market.InstrumentID, error) {
	return f.ids, f.err
}

func refreshRouter(services KernelServices) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return NewRouter(services)
}

func refreshRequest(t *testing.T, router *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/market/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func TestMarketRefreshTriggersFullScanWithoutIdentity(t *testing.T) {
	trigger := &refreshTriggerFake{}
	router := refreshRouter(KernelServices{MarketTrigger: trigger, MarketWorkers: 8})

	for _, body := range []string{"", "{}", "   "} {
		w := refreshRequest(t, router, body)
		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "ACCEPTED")
		require.Equal(t, 8, trigger.workers)
	}
}

func TestMarketRefreshMapsAlreadyRunningTrigger(t *testing.T) {
	router := refreshRouter(KernelServices{MarketTrigger: &refreshTriggerFake{err: application.ErrRefreshAlreadyRunning}})

	w := refreshRequest(t, router, "")
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "MARKET_REFRESH_ALREADY_RUNNING")
}

func TestMarketRefreshRefreshesSingleInstrument(t *testing.T) {
	ingestion := &refreshIngestionFake{}
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	router := refreshRouter(KernelServices{
		MarketIngestion: ingestion,
		Instruments:     &refreshLookupFake{ids: []market.InstrumentID{id}},
	})

	w := refreshRequest(t, router, `{"exchange":"SSE","code":"600000"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, id, ingestion.id)
	require.Contains(t, w.Body.String(), `"daily_bars":10`)
}

func TestMarketRefreshRejectsHalfIdentity(t *testing.T) {
	router := refreshRouter(KernelServices{})

	for _, body := range []string{`{"code":"600000"}`, `{"exchange":"SSE"}`} {
		w := refreshRequest(t, router, body)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "INVALID_REQUEST")
	}
}

func TestMarketRefreshMapsResolutionOutcomes(t *testing.T) {
	sseID := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	router := refreshRouter(KernelServices{
		MarketIngestion: &refreshIngestionFake{},
		Instruments:     &refreshLookupFake{ids: []market.InstrumentID{sseID, {Exchange: market.BSE, Code: "600000"}}},
	})

	w := refreshRequest(t, router, `{"exchange":"SSE","code":"600000"}`)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "AMBIGUOUS_INSTRUMENT")

	router = refreshRouter(KernelServices{
		MarketIngestion: &refreshIngestionFake{},
		Instruments:     &refreshLookupFake{},
	})
	w = refreshRequest(t, router, `{"exchange":"SSE","code":"600000"}`)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "NOT_FOUND")

	// 代码存在但唯一匹配的交易所与请求身份不符时，请求的证券视为不存在。
	router = refreshRouter(KernelServices{
		MarketIngestion: &refreshIngestionFake{},
		Instruments:     &refreshLookupFake{ids: []market.InstrumentID{sseID}},
	})
	w = refreshRequest(t, router, `{"exchange":"SZSE","code":"600000"}`)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "NOT_FOUND")
}

func TestMarketRefreshMapsRefreshFailure(t *testing.T) {
	router := refreshRouter(KernelServices{
		MarketIngestion: &refreshIngestionFake{err: application.ErrRefreshAlreadyRunning},
		Instruments:     &refreshLookupFake{ids: []market.InstrumentID{{Exchange: market.SSE, Code: "600000"}}},
	})

	w := refreshRequest(t, router, `{"exchange":"SSE","code":"600000"}`)
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "MARKET_REFRESH_ALREADY_RUNNING")
}

func TestMarketRefreshValidatesIdentity(t *testing.T) {
	router := refreshRouter(KernelServices{
		MarketIngestion: &refreshIngestionFake{},
		Instruments:     &refreshLookupFake{},
	})

	for _, body := range []string{`{"exchange":"NYSE","code":"600000"}`, `{"exchange":"SSE","code":"6000"}`, `{"exchange":"SSE","code":"600000","extra":1}`, `invalid`} {
		w := refreshRequest(t, router, body)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "INVALID_REQUEST")
	}
}

func TestMarketRefreshWithoutKernelReturnsInternalError(t *testing.T) {
	router := refreshRouter(KernelServices{})

	w := refreshRequest(t, router, "")
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	w = refreshRequest(t, router, `{"exchange":"SSE","code":"600000"}`)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
