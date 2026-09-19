package broker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"trading/internal/market"
)

func TestParseSinaDailyProducesFixedPointDailyBars(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	from := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	body := []byte(`[
		{"day":"2026-09-10","open":"10.120","high":"10.800","low":"9.990","close":"10.500","volume":"12345"},
		{"day":"2026-09-11","open":"10.500","high":"10.700","low":"10.200","close":"10.260","volume":"54321"}
	]`)

	bars, err := ParseSinaDaily(body, id, from, to)
	require.NoError(t, err)
	require.Len(t, bars, 2)
	require.Equal(t, market.Price(101_200), bars[0].Open)
	require.Equal(t, market.Price(108_000), bars[0].High)
	require.Equal(t, market.Price(99_900), bars[0].Low)
	require.Equal(t, market.Price(105_000), bars[0].Close)
	require.Equal(t, int64(12_345), bars[0].Volume)
	require.Equal(t, market.Day, bars[0].Timeframe)
	require.Equal(t, from, bars[0].CloseTime)
	require.Equal(t, market.DataVersion(0), bars[0].Version)
}

func TestParseSinaDailyRejectsDuplicateTradeDate(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	body := []byte(`[
		{"day":"2026-09-10","open":"10","high":"11","low":"9","close":"10","volume":"1"},
		{"day":"2026-09-10","open":"10","high":"11","low":"9","close":"10","volume":"1"}
	]`)

	_, err := ParseSinaDaily(body, id, at, at)
	require.ErrorIs(t, err, ErrMalformedResponse)
}

func TestParseSinaDailyRejectsEmptyAndInvalidOHLC(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

	_, err := ParseSinaDaily([]byte(`[]`), id, at, at)
	require.ErrorIs(t, err, ErrIncompleteData)

	_, err = ParseSinaDaily([]byte(`[{
		"day":"2026-09-10","open":"10","high":"9","low":"8","close":"10","volume":"1"
	}]`), id, at, at)
	require.ErrorIs(t, err, ErrMalformedResponse)

	_, err = ParseSinaDaily([]byte(`[{"day":"2026-09-10","open":"10","high":"11","low":"9","close":"10","volume":"1"}] trailing`), id, at, at)
	require.ErrorIs(t, err, ErrMalformedResponse)
}

func TestParseSinaDailyDiscardsOlderLookbackRows(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	from := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	body := []byte(`[
		{"day":"2026-09-09","open":"9","high":"10","low":"8","close":"9","volume":"1"},
		{"day":"2026-09-10","open":"10","high":"11","low":"9","close":"10","volume":"2"}
	]`)

	bars, err := ParseSinaDaily(body, id, from, to)
	require.NoError(t, err)
	require.Len(t, bars, 1)
	require.Equal(t, from, bars[0].CloseTime)
}

func TestParseSinaQFQInvertsDecimalFactorsAndSortsDates(t *testing.T) {
	body := []byte(`var sh600000qfq={"total":2,"data":[
		{"d":"2026-07-16","f":"1.0000000000000000"},
		{"d":"2025-07-16","f":"1.2500000000000000"}
	]}`)

	factors, err := ParseSinaQFQ(body)
	require.NoError(t, err)
	require.Equal(t, []market.AdjustmentFactor{
		{EffectiveTime: time.Date(2025, 7, 16, 0, 0, 0, 0, time.UTC), Numerator: 4, Denominator: 5},
		{EffectiveTime: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), Numerator: 1, Denominator: 1},
	}, factors)
}

func TestParseSinaQFQAcceptsTrailingBlockComment(t *testing.T) {
	body := []byte(`var sh600000qfq={"total":1,"data":[
		{"d":"2026-07-16","f":"1.0000000000000000"}
	]};
	/*vp7RmfDndcW0BFvsHcN3G2 */`)

	factors, err := ParseSinaQFQ(body)
	require.NoError(t, err)
	require.Equal(t, []market.AdjustmentFactor{
		{EffectiveTime: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), Numerator: 1, Denominator: 1},
	}, factors)
}

func TestParseSinaQFQRejectsMalformedPayloads(t *testing.T) {
	inputs := [][]byte{
		[]byte(`{"total":0,"data":[]}`),
		[]byte(`var x={"total":0,"data":[]}`),
		[]byte(`var xqfq={"total":1,"data":[]}`),
		[]byte(`var xqfq={"total":1,"data":[{"d":"2026-01-01","f":"0"}]}`),
		[]byte(`var xqfq={"total":1,"data":[{"d":"2026-01-01","f":"not-a-number"}]}`),
		[]byte(`var xqfq={"total":1,"data":[{"d":"2026-01-01","f":"1"}]} trailing`),
		[]byte(`var xqfq={"total":1,"data":[{"d":"2026-01-01","f":"1"}]};/*c*/ trailing`),
	}
	for _, input := range inputs {
		_, err := ParseSinaQFQ(input)
		require.True(t, errors.Is(err, ErrMalformedResponse) || errors.Is(err, ErrIncompleteData), "%s: %v", input, err)
	}
}

func TestSinaMarketSourceUsesDailyAndFactorContractsWithSharedPacing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "https://finance.sina.com.cn/", r.Header.Get("Referer"))
		switch r.URL.Path {
		case "/daily":
			require.Equal(t, "sh600000", r.URL.Query().Get("symbol"))
			require.Equal(t, "240", r.URL.Query().Get("scale"))
			require.Equal(t, "no", r.URL.Query().Get("ma"))
			require.Equal(t, "30", r.URL.Query().Get("datalen"))
			_, _ = w.Write([]byte(`[{"day":"2026-09-11","open":"10","high":"11","low":"9","close":"10","volume":"1"}]`))
		case "/factor/sh600000/qfq.js":
			_, _ = w.Write([]byte(`var sh600000qfq={"total":1,"data":[{"d":"2026-09-11","f":"1"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	starts := &requestStartRecorder{base: client.Transport}
	client.Transport = starts
	pacer := rate.NewLimiter(rate.Every(20*time.Millisecond), 1)
	source := NewSinaMarketSourceWithClient(client, pacer, server.URL+"/daily", server.URL+"/factor")
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	bars, err := source.FetchDailyBars(context.Background(), id, at, at)
	require.NoError(t, err)
	require.Len(t, bars, 1)
	factors, err := source.FetchAdjustmentFactors(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, factors, 1)

	times := starts.Times()
	require.Len(t, times, 2)
	require.GreaterOrEqual(t, times[1].Sub(times[0]), 15*time.Millisecond)
}

func TestSinaMarketSourceRetriesServerFailureThroughSharedPacing(t *testing.T) {
	var mu sync.Mutex
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		attempt := attempts
		mu.Unlock()
		if attempt == 1 {
			http.Error(w, "temporary detail that must not escape", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[{"day":"2026-09-11","open":"10","high":"11","low":"9","close":"10","volume":"1"}]`))
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	starts := &requestStartRecorder{base: client.Transport}
	client.Transport = starts
	source := NewSinaMarketSourceWithClient(client, rate.NewLimiter(rate.Every(20*time.Millisecond), 1), server.URL, server.URL)
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	bars, err := source.FetchDailyBars(context.Background(), id, at, at)
	require.NoError(t, err)
	require.Len(t, bars, 1)

	times := starts.Times()
	require.Len(t, times, 2)
	require.GreaterOrEqual(t, times[1].Sub(times[0]), 15*time.Millisecond)
}

func TestSinaMarketSourceHonorsRetryAfter(t *testing.T) {
	var mu sync.Mutex
	var arrivals []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		attempt := len(arrivals)
		mu.Unlock()
		if attempt == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`[{"day":"2026-09-11","open":"10","high":"11","low":"9","close":"10","volume":"1"}]`))
	}))
	t.Cleanup(server.Close)

	source := NewSinaMarketSourceWithClient(server.Client(), rate.NewLimiter(rate.Inf, 1), server.URL, server.URL)
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	_, err := source.FetchDailyBars(context.Background(), id, at, at)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, arrivals, 2)
	require.GreaterOrEqual(t, arrivals[1].Sub(arrivals[0]), 950*time.Millisecond)
}

func TestSinaMarketSourceRetriesResponseBodyReadFailure(t *testing.T) {
	var calls int
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(failingReader{})}, nil
		}
		body := `[{"day":"2026-09-11","open":"10","high":"11","low":"9","close":"10","volume":"1"}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	source := NewSinaMarketSourceWithClient(client, rate.NewLimiter(rate.Inf, 1), "https://example.com/daily", "https://example.com/factor")
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	_, err := source.FetchDailyBars(context.Background(), id, at, at)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type requestStartRecorder struct {
	base   http.RoundTripper
	mu     sync.Mutex
	starts []time.Time
}

func (r *requestStartRecorder) RoundTrip(request *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.starts = append(r.starts, time.Now())
	r.mu.Unlock()
	return r.base.RoundTrip(request)
}

func (r *requestStartRecorder) Times() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Time(nil), r.starts...)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
