package broker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"trading/internal/market"
)

func TestSinaFuturesSourceParsesMainDailyBars(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}
	bars, err := ParseSinaFuturesDaily(
		readSinaFuturesFixture(t), id,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 14, 23, 59, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	require.Len(t, bars, 2)
	require.Equal(t, market.Price(9_400_000), bars[0].Open)
	require.Equal(t, market.Price(9_449_400), bars[1].Close)
	require.Equal(t, int64(104800), bars[1].Volume)
	require.Equal(t, id, bars[1].Instrument)
	require.Equal(t, market.Day, bars[1].Timeframe)
	require.Equal(t, time.Date(2026, 9, 11, 7, 0, 0, 0, time.UTC), bars[1].CloseTime)
}

func TestSinaFuturesSourceSupportsConfiguredMainProductsOnly(t *testing.T) {
	want := map[market.InstrumentID]string{
		{Exchange: market.SHFE, Code: "AU.MAIN"}: "AU0",
		{Exchange: market.SHFE, Code: "AG.MAIN"}: "AG0",
		{Exchange: market.SHFE, Code: "FU.MAIN"}: "FU0",
		{Exchange: market.INE, Code: "SC.MAIN"}:  "SC0",
		{Exchange: market.INE, Code: "LU.MAIN"}:  "LU0",
		{Exchange: market.DCE, Code: "J.MAIN"}:   "J0",
		{Exchange: market.DCE, Code: "JM.MAIN"}:  "JM0",
		{Exchange: market.CZCE, Code: "ZC.MAIN"}: "ZC0",
	}
	for id, symbol := range want {
		got, err := sinaFuturesSymbol(id)
		require.NoError(t, err)
		require.Equal(t, symbol, got)
	}
	for _, id := range []market.InstrumentID{
		{Exchange: market.SHFE, Code: "AU202612"},
		{Exchange: market.SHFE, Code: "SC.MAIN"},
		{Exchange: market.SSE, Code: "600000"},
	} {
		_, err := sinaFuturesSymbol(id)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
}

func TestSinaFuturesSourceUsesOneFixedRequestAndSharedLimiter(t *testing.T) {
	var calls atomic.Int32
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		path = r.URL.EscapedPath()
		require.Equal(t, "AU0", r.URL.Query().Get("symbol"))
		require.Equal(t, "2026_9_14", r.URL.Query().Get("type"))
		_, _ = w.Write(readSinaFuturesFixture(t))
	}))
	defer server.Close()
	source := NewSinaFuturesSourceWithClient(server.Client(), rate.NewLimiter(rate.Inf, 1), server.URL)
	bars, err := source.FetchDailyBars(context.Background(), market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, bars, 2)
	require.Equal(t, int32(1), calls.Load())
	require.Contains(t, path, "InnerFuturesNewService.getDailyKLine")

	factors, err := source.FetchAdjustmentFactors(context.Background(), market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"})
	require.NoError(t, err)
	require.Equal(t, []market.AdjustmentFactor{{EffectiveTime: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Numerator: 1, Denominator: 1}}, factors)
}

func TestSinaFuturesParserRejectsMalformedPayload(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	for _, payload := range [][]byte{
		[]byte(`not jsonp`),
		[]byte(`var x=([{"d":"2026-09-11","o":"1","h":"2","l":"1","c":"1","v":"1","p":"1"}]);`),
		[]byte(`var x=([{"d":"2026-09-11","o":"1","h":"2","l":"1","c":"1","v":"bad","p":"1","s":"1"}]);`),
		[]byte(`var x=([{"d":"2026-09-11","o":"1","h":"2","l":"1","c":"1","v":"1","p":"1","s":"1"},{"d":"2026-09-11","o":"1","h":"2","l":"1","c":"1","v":"1","p":"1","s":"1"}]);`),
		[]byte(`var _AU02026_9_14=([{"d":"2026-09-11","o":"1","h":"2","l":"1","c":"1","v":"1","p":"1","s":"1"}]); trailing`),
		[]byte(`var _AG02026_9_14=([{"d":"2026-09-11","o":"1","h":"2","l":"1","c":"1","v":"1","p":"1","s":"1"}]);`),
		[]byte(`var _AU02026_9_14=([{"d":"2026-09-15","o":"1","h":"2","l":"1","c":"1","v":"1","p":"1","s":"1"}]);`),
	} {
		_, err := ParseSinaFuturesDaily(payload, id, from, to)
		require.True(t, errors.Is(err, ErrMalformedResponse) || errors.Is(err, ErrIncompleteData), "%s: %v", payload, err)
	}
}

func readSinaFuturesFixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "sina_futures_daily.js"))
	require.NoError(t, err)
	return body
}
