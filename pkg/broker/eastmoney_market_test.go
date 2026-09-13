package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"trading/internal/market"
)

func TestParseEastmoneyRawAndQFQProducesCompressedFactors(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	bars, factors, err := ParseEastmoneyKlines(
		readEastmoneyFixture(t, "eastmoney_kline_raw.json"),
		readEastmoneyFixture(t, "eastmoney_kline_qfq.json"),
		id,
		market.Day,
	)
	if err != nil {
		t.Fatalf("ParseEastmoneyKlines() error = %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(bars))
	}
	if bars[0].Open != market.Price(100_000) || bars[1].Amount != market.Money(400_000_000) {
		t.Fatalf("unexpected fixed-point bars: %+v, %+v", bars[0], bars[1])
	}
	if len(factors) != 1 {
		t.Fatalf("factors = %d, want compressed single factor", len(factors))
	}
	if factors[0].Numerator != 19 || factors[0].Denominator != 20 {
		t.Errorf("factor = %d/%d, want 19/20", factors[0].Numerator, factors[0].Denominator)
	}
	if !factors[0].EffectiveTime.Equal(bars[0].CloseTime) {
		t.Errorf("effective time = %s, want first bar close %s", factors[0].EffectiveTime, bars[0].CloseTime)
	}
	if factors[0].Version != 0 {
		t.Errorf("source version = %d, want 0", factors[0].Version)
	}
}

func TestParseEastmoneyKlinesAssignsExecutableTradingSessions(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	for _, tc := range []struct {
		name      string
		timeframe market.Timeframe
		raw       []byte
		qfq       []byte
		wantOpen  time.Time
		wantClose time.Time
	}{
		{name: "daily", timeframe: market.Day, raw: []byte(`{"rc":0,"data":{"klines":["2024-01-02,10,10,10,10,1,10"]}}`), qfq: []byte(`{"rc":0,"data":{"klines":["2024-01-02,10,10,10,10,1,10"]}}`), wantOpen: time.Date(2024, time.January, 2, 1, 30, 0, 0, time.UTC), wantClose: time.Date(2024, time.January, 2, 7, 0, 0, 0, time.UTC)},
		{name: "weekly", timeframe: market.Week, raw: []byte(`{"rc":0,"data":{"klines":["2024-01-05,10,10,10,10,1,10"]}}`), qfq: []byte(`{"rc":0,"data":{"klines":["2024-01-05,10,10,10,10,1,10"]}}`), wantOpen: time.Date(2024, time.January, 1, 1, 30, 0, 0, time.UTC), wantClose: time.Date(2024, time.January, 5, 7, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bars, _, err := ParseEastmoneyKlines(tc.raw, tc.qfq, id, tc.timeframe)
			if err != nil {
				t.Fatalf("ParseEastmoneyKlines() error = %v", err)
			}
			if len(bars) != 1 {
				t.Fatalf("bars = %d, want 1", len(bars))
			}
			if !bars[0].OpenTime.Equal(tc.wantOpen) || !bars[0].CloseTime.Equal(tc.wantClose) {
				t.Fatalf("session = %s..%s, want %s..%s", bars[0].OpenTime, bars[0].CloseTime, tc.wantOpen, tc.wantClose)
			}
		})
	}
}

func TestParseEastmoneyKlinesRejectsInconsistentOrPartialSeries(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	raw := readEastmoneyFixture(t, "eastmoney_kline_raw.json")
	for _, tc := range []struct {
		name string
		qfq  []byte
	}{
		{"mismatched adjustment", []byte(`{"rc":0,"data":{"klines":["2024-01-02,8.20,8.00,8.30,7.80,1000,8000,0,0,0,0","2024-01-03,19.00,19.00,19.38,18.62,2000,38000,0,0,0,0"]}}`)},
		{"missing date", []byte(`{"rc":0,"data":{"klines":["2024-01-02,9.50,9.50,9.69,9.31,1000,9500,0,0,0,0"]}}`)},
		{"duplicate date", []byte(`{"rc":0,"data":{"klines":["2024-01-02,9.50,9.50,9.69,9.31,1000,9500,0,0,0,0","2024-01-02,19.00,19.00,19.38,18.62,2000,38000,0,0,0,0"]}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bars, factors, err := ParseEastmoneyKlines(raw, tc.qfq, id, market.Day)
			if !errors.Is(err, ErrIncompleteData) {
				t.Fatalf("error = %v, want ErrIncompleteData", err)
			}
			if bars != nil || factors != nil {
				t.Fatalf("got partial publishable result: bars=%v factors=%v", bars, factors)
			}
		})
	}
}

func TestParseEastmoneyKlinesSortsBeforeCompressingFactors(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	raw := []byte(`{"rc":0,"data":{"klines":["2024-01-02,10,10,10,10,1,10","2024-01-04,10,10,10,10,1,10","2024-01-03,10,10,10,10,1,10"]}}`)
	qfq := []byte(`{"rc":0,"data":{"klines":["2024-01-02,8,8,8,8,1,8","2024-01-04,8,8,8,8,1,8","2024-01-03,9,9,9,9,1,9"]}}`)
	_, factors, err := ParseEastmoneyKlines(raw, qfq, id, market.Day)
	if err != nil {
		t.Fatalf("ParseEastmoneyKlines() error = %v", err)
	}
	if len(factors) != 3 || factors[0].Numerator != 4 || factors[1].Numerator != 9 || factors[2].Numerator != 4 {
		t.Fatalf("chronologically compressed factors = %+v, want 4/5, 9/10, 4/5", factors)
	}
	if !factors[0].EffectiveTime.Before(factors[1].EffectiveTime) || !factors[1].EffectiveTime.Before(factors[2].EffectiveTime) {
		t.Fatalf("factors are not strictly chronological: %+v", factors)
	}
}

func TestEastmoneyMarketSourceFetchBarsUsesExchangeAndTimeframeParameters(t *testing.T) {
	var mu sync.Mutex
	var got []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Query())
		mu.Unlock()
		if r.URL.Path != "/kline" {
			t.Fatalf("path = %s, want /kline", r.URL.Path)
		}
		if r.URL.Query().Get("fqt") == "0" {
			_, _ = w.Write(readEastmoneyFixture(t, "eastmoney_kline_raw.json"))
			return
		}
		_, _ = w.Write(readEastmoneyFixture(t, "eastmoney_kline_qfq.json"))
	}))
	defer srv.Close()

	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL+"/kline", srv.URL+"/data")
	from := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC)
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SZSE, "000001"), market.Week, from, to)
	if err != nil {
		t.Fatalf("FetchBars() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("requests = %d, want one raw and one QFQ", len(got))
	}
	for _, query := range got {
		if query.Get("secid") != "0.000001" || query.Get("klt") != "102" || query.Get("beg") != "20240102" || query.Get("end") != "20240103" {
			t.Errorf("unexpected query: %v", query)
		}
	}
	if got[0].Get("fqt") == got[1].Get("fqt") {
		t.Errorf("fqt values = %q and %q, want 0 and 1", got[0].Get("fqt"), got[1].Get("fqt"))
	}
}

func TestEastmoneyMarketSourceClassifiesHTTPAndResponseErrors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		want        error
		retryAfter  string
		wantRetryIn time.Duration
	}{
		{"non 2xx", http.StatusBadGateway, `{}`, ErrUpstream, "3", 3 * time.Second},
		{"success false", http.StatusOK, `{"success":false}`, ErrUpstream, "", 0},
		{"malformed", http.StatusOK, `{"rc":0,"data":{"klines":["bad"]}}`, ErrMalformedResponse, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
			_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.wantRetryIn > 0 {
				var upstream *UpstreamError
				if !errors.As(err, &upstream) || !upstream.HasRetryAfter || upstream.RetryAfter != tc.wantRetryIn {
					t.Fatalf("Retry-After metadata = %#v, want %s", upstream, tc.wantRetryIn)
				}
			}
		})
	}
}

func TestEastmoneyMarketSourcePreservesCancellationAndClassifiesTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()

	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: 30 * time.Millisecond}, srv.URL, srv.URL)
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, ErrUpstreamTimeout) {
		t.Fatalf("timeout error = %v, want ErrUpstreamTimeout", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = source.FetchBars(ctx, eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrRequestCanceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled and ErrRequestCanceled", err)
	}
}

func TestEastmoneyMarketSourceMakesNoProviderRetriesOrOversizedReads(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, ErrUpstream) || requests != 1 {
		t.Fatalf("error=%v requests=%d, want one upstream attempt", err, requests)
	}

	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rc":0,"data":{"klines":["`))
		_, _ = w.Write(make([]byte, eastmoneyResponseLimit+1))
	}))
	defer large.Close()
	source = NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, large.URL, large.URL)
	_, _, err = source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("oversized body error = %v, want ErrMalformedResponse", err)
	}
}

func TestParseEastmoneyKlinesRejectsMalformedFixedPointAndTradingStatus(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	for _, tc := range []struct {
		name string
		body string
	}{
		{"non decimal", `{"rc":0,"data":{"klines":["2024-01-02,NaN,10.00,10.20,9.80,1000,10000"]}}`},
		{"fraction below scale", `{"rc":0,"data":{"klines":["2024-01-02,10.00001,10.00,10.20,9.80,1000,10000"]}}`},
		{"inconsistent suspension", `{"rc":0,"data":{"klines":["2024-01-02,10.00,10.00,10.20,9.80,0,10000"]}}`},
		{"missing payload", `{"rc":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bars, factors, err := ParseEastmoneyKlines([]byte(tc.body), []byte(tc.body), id, market.Day)
			if !errors.Is(err, ErrMalformedResponse) || bars != nil || factors != nil {
				t.Fatalf("error=%v bars=%v factors=%v, want malformed empty result", err, bars, factors)
			}
		})
	}
}

func TestEastmoneyMarketSourcePaginatesAndSplitsSameDayActions(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("pageNumber") == "1" {
			rows := `{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.20","BONUS_RATIO":"1.50","IT_RATIO":"0.50"}` + "," + eastmoneyDistinctZeroActionRows(eastmoneyActionPageSize-1)
			_, _ = w.Write(eastmoneyActionPageJSON(501, 2, 1, rows))
			return
		}
		_, _ = w.Write(eastmoneyActionPageJSON(501, 2, 2, `{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-07-01","PRETAX_BONUS_RMB":"1.00","BONUS_RATIO":"0","IT_RATIO":"0"}`))
	}))
	defer srv.Close()

	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
	actions, err := source.FetchCorporateActions(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"))
	if err != nil {
		t.Fatalf("FetchCorporateActions() error = %v", err)
	}
	if requests != 2 || len(actions) != 3 {
		t.Fatalf("requests=%d actions=%d, want 2 pages and 3 actions", requests, len(actions))
	}
	if actions[0].Kind != market.CashDividend || actions[0].CashPerShare != market.Money(4_200) {
		t.Errorf("cash action = %+v, want 10派4.2 converted to 0.42", actions[0])
	}
	if actions[1].Kind != market.ShareDistribution || actions[1].ShareNumerator != 6 || actions[1].ShareDenominator != 5 {
		t.Errorf("share action = %+v, want 12/10 reduced to 6/5", actions[1])
	}
	if actions[0].ID == actions[1].ID || !actions[0].ExDate.Before(actions[2].ExDate) {
		t.Errorf("actions are not independently identified and stably sorted: %+v", actions)
	}
}

func TestEastmoneyMarketSourceRejectsMalformedCorporateAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"code":0,"result":{"count":1,"pages":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"","PRETAX_BONUS_RMB":"0","BONUS_RATIO":"0","IT_RATIO":"0"}]}}`))
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
	_, err := source.FetchCorporateActions(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"))
	if !errors.Is(err, ErrMalformedResponse) || !strings.Contains(err.Error(), "ex-dividend date") {
		t.Fatalf("error = %v, want invalid ex-dividend date", err)
	}
}

func TestEastmoneyCorporateActionEnvelopeRejectsInvalidRowsAndPagination(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	for _, tc := range []struct {
		name     string
		body     string
		want     error
		contains string
	}{
		{"false success", `{"success":false,"code":1,"result":{}}`, ErrUpstream, ""},
		{"nonzero code", `{"success":true,"code":3,"result":{}}`, ErrUpstream, ""},
		{"missing pages", `{"success":true,"code":0,"result":{"data":[]}}`, ErrMalformedResponse, "count, pages and data"},
		{"negative ratio", `{"success":true,"code":0,"result":{"count":1,"pages":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"0","BONUS_RATIO":"-1","IT_RATIO":"0"}]}}`, ErrMalformedResponse, "invalid bonus ratio"},
		{"wrong code", `{"success":true,"code":0,"result":{"count":1,"pages":1,"data":[{"SECURITY_CODE":"000001","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"0","BONUS_RATIO":"0","IT_RATIO":"0"}]}}`, ErrMalformedResponse, "invalid security code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseEastmoneyCorporateActions([]byte(tc.body), id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.contains != "" && !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error = %v, want text %q", err, tc.contains)
			}
		})
	}
}

func TestEastmoneyMarketSourceValidatesExchangeTimeframeAndRetryAfterDate(t *testing.T) {
	for _, tc := range []struct {
		id market.InstrumentID
		tf market.Timeframe
	}{
		{eastmoneyInstrument(t, market.SSE, "600000"), market.Day},
		{eastmoneyInstrument(t, market.BSE, "430047"), market.Month},
	} {
		secid, err := eastmoneySecurityID(tc.id)
		if err != nil || secid == "" || eastmoneyKlinePeriod(tc.tf) == "" {
			t.Fatalf("mapping id=%+v tf=%d: secid=%q err=%v", tc.id, tc.tf, secid, err)
		}
	}
	source := NewEastmoneyMarketSource()
	if source.client.Timeout != eastmoneyHTTPTimeout || source.klineBaseURL != eastmoneyKlineURL || source.dataCenterBaseURL != eastmoneyDataCenterURL {
		t.Fatalf("default source = %+v", source)
	}
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.UnknownTimeframe, eastmoneyDate(3), eastmoneyDate(2))
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("invalid request error = %v", err)
	}
	when := time.Now().Add(time.Second).UTC().Format(http.TimeFormat)
	if _, ok := parseRetryAfter(when, time.Now()); !ok {
		t.Fatal("HTTP-date Retry-After was not parsed")
	}
}

func TestEastmoneyLowLevelParsingRejectsEveryInvalidEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want error
	}{
		{"invalid json", `{`, ErrMalformedResponse},
		{"failed response", `{"success":false}`, ErrUpstream},
		{"failed rc", `{"rc":1,"data":{"klines":[]}}`, ErrUpstream},
		{"null data", `{"rc":0,"data":null}`, ErrMalformedResponse},
		{"invalid data", `{"rc":0,"data":"bad"}`, ErrMalformedResponse},
		{"missing klines", `{"rc":0,"data":{}}`, ErrMalformedResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseEastmoneyKlineResponse([]byte(tc.body))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
	for _, record := range []string{
		"bad-date,10,10,10,10,1,1",
		"2024-01-02,10,bad,10,10,1,1",
		"2024-01-02,10,10,bad,10,1,1",
		"2024-01-02,10,10,10,bad,1,1",
		"2024-01-02,10,10,10,10,bad,1",
		"2024-01-02,10,10,10,10,1,bad",
		"2024-01-02,10,10,9,10,1,1",
	} {
		if _, err := parseEastmoneyKline(record); !errors.Is(err, ErrMalformedResponse) {
			t.Fatalf("record %q error = %v", record, err)
		}
	}
	suspended, err := parseEastmoneyKline("2024-01-02,10,10,10,10,0,0")
	if err != nil || suspended.trading != market.Suspended {
		t.Fatalf("suspended status = %+v, %v", suspended, err)
	}
	if _, _, err := ParseEastmoneyKlines([]byte(`{"rc":0,"data":{"klines":[]}}`), []byte(`{"rc":0,"data":{"klines":[]}}`), market.InstrumentID{}, market.Day); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("invalid instrument error = %v", err)
	}
	if _, _, err := ParseEastmoneyKlines([]byte(`{"rc":0,"data":{"klines":[]}}`), []byte(`{"rc":0,"data":{"klines":[]}}`), eastmoneyInstrument(t, market.SSE, "600000"), market.UnknownTimeframe); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("invalid timeframe error = %v", err)
	}
}

func TestEastmoneyFixedPointAndTransportHelpers(t *testing.T) {
	if value, err := parseEastmoneyScaled("12.3456", market.ValueScale); err != nil || value != 123456 {
		t.Fatalf("scaled decimal = %d, %v", value, err)
	}
	for _, value := range []string{"", "1e3", "NaN", "0.00001"} {
		if _, err := parseEastmoneyScaled(value, market.ValueScale); err == nil {
			t.Fatalf("parseEastmoneyScaled(%q) unexpectedly succeeded", value)
		}
	}
	for _, value := range []json.RawMessage{[]byte(`null`), []byte(`""`), []byte(`"oops"`)} {
		if _, err := rawJSONDecimal(value); err == nil {
			t.Fatalf("rawJSONDecimal(%s) unexpectedly succeeded", value)
		}
	}
	if decimal, err := rawJSONDecimal([]byte(`1.25`)); err != nil || decimal.Cmp(bigRat(t, "1.25")) != 0 {
		t.Fatalf("numeric JSON decimal = %v, %v", decimal, err)
	}
	if _, err := rawJSONString([]byte(`null`)); err == nil {
		t.Fatal("null JSON string unexpectedly succeeded")
	}
	if got, err := rawJSONString([]byte(`" 600000 "`)); err != nil || got != "600000" {
		t.Fatalf("raw JSON string = %q, %v", got, err)
	}
	factor, err := eastmoneyFactor(3, 1)
	if err != nil || factor.Numerator != 33333333 || factor.Denominator != 100000000 {
		t.Fatalf("rounded factor = %+v, %v", factor, err)
	}
	if _, err := eastmoneyFactor(0, 1); err == nil {
		t.Fatal("zero raw close unexpectedly produced a factor")
	}
	if _, _, ok := ratNumeratorDenominator(bigRat(t, "9223372036854775808")); ok {
		t.Fatal("oversized rational unexpectedly fit int64")
	}
	if _, ok := ratInt64(bigRat(t, "0.5")); ok {
		t.Fatal("fractional rational unexpectedly fit fixed point")
	}
	if gcd64(-6, 4) != 2 || roundPositiveRat(bigRat(t, "1.5")).Int64() != 2 {
		t.Fatal("fixed point arithmetic helper returned an unexpected result")
	}
	if _, ok := parseRetryAfter("not-a-date", time.Now()); ok {
		t.Fatal("invalid Retry-After unexpectedly parsed")
	}
	if duration, ok := parseRetryAfter("Sun, 06 Nov 1994 08:49:37 GMT", time.Now()); !ok || duration != 0 {
		t.Fatalf("past Retry-After = %s, %t", duration, ok)
	}
	if !errors.Is(classifyEastmoneyRequestError(eastmoneyTimeoutError{}), ErrUpstreamTimeout) {
		t.Fatal("network timeout was not classified")
	}
	if !errors.Is(classifyEastmoneyRequestError(errors.New("connection reset")), ErrUpstream) {
		t.Fatal("network failure was not classified")
	}
}

func TestEastmoneyCorporateActionSupportsNumericFieldsAndRejectsInvalidPrecision(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	numeric := []byte(`{"success":true,"code":0,"result":{"count":1,"pages":1,"pageNumber":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13 00:00:00","PRETAX_BONUS_RMB":1.2,"BONUS_RATIO":0,"IT_RATIO":1.25}]}}`)
	actions, err := ParseEastmoneyCorporateActions(numeric, id)
	if err != nil || len(actions) != 2 || actions[0].CashPerShare != 1200 || actions[1].ShareNumerator != 9 || actions[1].ShareDenominator != 8 {
		t.Fatalf("numeric action parsing: actions=%+v err=%v", actions, err)
	}
	for _, body := range [][]byte{
		[]byte(`{"success":true,"code":0,"result":{"pages":1,"data":null}}`),
		[]byte(`{"success":true,"code":0,"result":{"pages":1,"data":{}}}`),
	} {
		_, err := ParseEastmoneyCorporateActions(body, id)
		if !errors.Is(err, ErrMalformedResponse) {
			t.Fatalf("body %s error = %v", body, err)
		}
	}
	_, err = ParseEastmoneyCorporateActions([]byte(`{"success":true,"code":0,"result":{"count":1,"pages":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"0.0001","BONUS_RATIO":"0","IT_RATIO":"0"}]}}`), id)
	if !errors.Is(err, ErrMalformedResponse) || !strings.Contains(err.Error(), "cash ratio is below fixed-point precision") {
		t.Fatalf("sub-tick cash ratio error = %v", err)
	}
	if _, err := ParseEastmoneyCorporateActions([]byte(`{"success":true,"code":0,"result":{"pages":0,"data":[]}}`), market.InstrumentID{}); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("invalid action instrument error = %v", err)
	}
}

func TestEastmoneyMarketSourceRejectsChangingOrExcessivePagination(t *testing.T) {
	for _, tc := range []struct {
		name string
		page func(int) string
		want error
	}{
		{
			name: "excessive pages",
			page: func(int) string {
				return string(eastmoneyActionPageJSON(5_000_001, 10_001, 1, eastmoneyDistinctZeroActionRows(eastmoneyActionPageSize)))
			},
			want: ErrMalformedResponse,
		},
		{
			name: "changed pages",
			page: func(page int) string {
				if page == 1 {
					return string(eastmoneyActionPageJSON(501, 2, 1, eastmoneyDistinctZeroActionRows(eastmoneyActionPageSize)))
				}
				return string(eastmoneyActionPageJSON(1, 1, 1, eastmoneyDistinctZeroActionRows(1)))
			},
			want: ErrIncompleteData,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.page(intQuery(t, r.URL.Query(), "pageNumber"))))
			}))
			defer srv.Close()
			source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
			_, err := source.FetchCorporateActions(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func eastmoneyInstrument(t *testing.T, exchange market.Exchange, code string) market.InstrumentID {
	t.Helper()
	id := market.InstrumentID{Exchange: exchange, Code: code}
	if err := id.Validate(); err != nil {
		t.Fatalf("invalid fixture instrument: %v", err)
	}
	return id
}

func eastmoneyDate(day int) time.Time {
	return time.Date(2024, time.January, day, 0, 0, 0, 0, time.UTC)
}

func readEastmoneyFixture(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return contents
}

func bigRat(t *testing.T, value string) *big.Rat {
	t.Helper()
	ratio, ok := new(big.Rat).SetString(value)
	if !ok {
		t.Fatalf("invalid test ratio %q", value)
	}
	return ratio
}

func intQuery(t *testing.T, query url.Values, name string) int {
	t.Helper()
	value, err := strconv.Atoi(query.Get(name))
	if err != nil {
		t.Fatalf("query %s = %q: %v", name, query.Get(name), err)
	}
	return value
}

func eastmoneyActionPageJSON(count, pages, page int, rows string) []byte {
	return []byte(fmt.Sprintf(`{"success":true,"code":0,"result":{"count":%d,"pages":%d,"pageNumber":%d,"data":[%s]}}`, count, pages, page, rows))
}

func eastmoneyActionPageJSONWithoutNumber(count, pages int, rows string) []byte {
	return []byte(fmt.Sprintf(`{"success":true,"code":0,"result":{"count":%d,"pages":%d,"data":[%s]}}`, count, pages, rows))
}

func eastmoneyZeroActionRows(count int) string {
	return strings.TrimPrefix(strings.Repeat(`,{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-14","PRETAX_BONUS_RMB":"0","BONUS_RATIO":"0","IT_RATIO":"0"}`, count), ",")
}

func eastmoneyDistinctZeroActionRows(count int) string {
	rows := make([]string, count)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"ID":"zero-%d","SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-14","PRETAX_BONUS_RMB":"0","BONUS_RATIO":"0","IT_RATIO":"0"}`, i)
	}
	return strings.Join(rows, ",")
}

type eastmoneyTimeoutError struct{}

func (eastmoneyTimeoutError) Error() string   { return "timeout" }
func (eastmoneyTimeoutError) Timeout() bool   { return true }
func (eastmoneyTimeoutError) Temporary() bool { return true }

func TestParseEastmoneyDividendFixtureConvertsPerTenShares(t *testing.T) {
	actions, err := ParseEastmoneyCorporateActions(readEastmoneyFixture(t, "eastmoney_dividend.json"), eastmoneyInstrument(t, market.SSE, "600000"))
	if err != nil {
		t.Fatalf("ParseEastmoneyCorporateActions() error = %v", err)
	}
	if len(actions) != 2 || actions[0].CashPerShare != market.Money(4_200) || actions[1].ShareNumerator != 6 || actions[1].ShareDenominator != 5 {
		t.Fatalf("actions = %+v", actions)
	}
	if !reflect.DeepEqual(actions[0].Instrument, eastmoneyInstrument(t, market.SSE, "600000")) {
		t.Errorf("instrument = %+v", actions[0].Instrument)
	}
}

func TestParseEastmoneyRealResponseShapeAllowsMissingPageNumber(t *testing.T) {
	actions, err := ParseEastmoneyCorporateActions(readEastmoneyFixture(t, "eastmoney_dividend.json"), eastmoneyInstrument(t, market.SSE, "600000"))
	if err != nil || len(actions) != 2 {
		t.Fatalf("real response shape actions=%+v err=%v", actions, err)
	}
}

func TestEastmoneyCorporateActionsTreatsFirstPage9201AsEmptyOnly(t *testing.T) {
	noData := readEastmoneyFixture(t, "eastmoney_dividend_empty.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(noData)
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
	actions, err := source.FetchCorporateActions(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"))
	if err != nil || actions != nil {
		t.Fatalf("first page 9201 actions=%v err=%v", actions, err)
	}

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageNumber") == "1" {
			_, _ = w.Write(eastmoneyActionPageJSON(501, 2, 1, eastmoneyDistinctZeroActionRows(eastmoneyActionPageSize)))
			return
		}
		_, _ = w.Write(noData)
	}))
	defer srv.Close()
	source = NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
	actions, err = source.FetchCorporateActions(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"))
	if !errors.Is(err, ErrIncompleteData) || actions != nil {
		t.Fatalf("later 9201 actions=%v err=%v, want ErrIncompleteData", actions, err)
	}
}

func TestEastmoneyCorporateAction9201RequiresExplicitFalseSuccess(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	for _, body := range [][]byte{
		[]byte(`{"code":9201,"result":null}`),
		[]byte(`{"success":null,"code":9201,"result":null}`),
	} {
		actions, err := ParseEastmoneyCorporateActions(body, id)
		if !errors.Is(err, ErrMalformedResponse) || actions != nil {
			t.Fatalf("9201 without explicit false success: actions=%v err=%v", actions, err)
		}
	}
}

func TestEastmoneyCorporateActionsTracksRequestedPageWhenResponseOmitsPageNumber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageNumber") == "1" {
			_, _ = w.Write(eastmoneyActionPageJSONWithoutNumber(501, 2, eastmoneyDistinctZeroActionRows(eastmoneyActionPageSize)))
			return
		}
		_, _ = w.Write(eastmoneyActionPageJSONWithoutNumber(501, 2, `{"ID":"last","SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-15","PRETAX_BONUS_RMB":"1","BONUS_RATIO":"0","IT_RATIO":"0"}`))
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL, srv.URL)
	actions, err := source.FetchCorporateActions(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"))
	if err != nil || len(actions) != 1 {
		t.Fatalf("missing pageNumber actions=%+v err=%v", actions, err)
	}
}

func TestEastmoneyCorporateActionRejectsRepeatedSourceRecordsBeforeConversion(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	row := `{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"0","BONUS_RATIO":"0","IT_RATIO":"0"}`
	actions, err := ParseEastmoneyCorporateActions(eastmoneyActionPageJSON(2, 1, 1, row+","+row), id)
	if !errors.Is(err, ErrIncompleteData) || actions != nil {
		t.Fatalf("duplicate zero records actions=%v err=%v", actions, err)
	}

	body := eastmoneyActionPageJSON(2, 1, 1, `{"ID":"a","SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"1","BONUS_RATIO":"0","IT_RATIO":"0"},{"ID":"a ","SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"1","BONUS_RATIO":"0","IT_RATIO":"0"}`)
	actions, err = ParseEastmoneyCorporateActions(body, id)
	if err != nil || len(actions) != 2 || actions[0].ID == actions[1].ID {
		t.Fatalf("raw source identities were normalized: actions=%+v err=%v", actions, err)
	}
}

func TestEastmoneyURLNormalizationPreservesQuerySlashAndPathOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nested" || r.URL.Query().Get("fixed") != "abc/" {
			t.Fatalf("path/query = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("fqt") == "0" {
			_, _ = w.Write(readEastmoneyFixture(t, "eastmoney_kline_raw.json"))
			return
		}
		_, _ = w.Write(readEastmoneyFixture(t, "eastmoney_kline_qfq.json"))
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL+"/nested/?fixed=abc/", srv.URL)
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if err != nil {
		t.Fatalf("normalized URL request: %v", err)
	}
}

func TestParseRetryAfterSaturatesDecimalBeyondInt64(t *testing.T) {
	duration, ok := parseRetryAfter(strings.Repeat("9", 100_000), time.Now())
	if !ok || duration != time.Duration(1<<63-1) {
		t.Fatalf("oversized Retry-After = %s, %t", duration, ok)
	}
}

func TestParseRetryAfterStripsLeadingZeroesBeforeRangeCheck(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{value: strings.Repeat("0", 100_000) + "1", want: time.Second},
		{value: strings.Repeat("0", 100_000), want: 0},
		{value: strings.Repeat("0", 100_000) + strings.Repeat("9", 20), want: time.Duration(1<<63 - 1)},
	} {
		duration, ok := parseRetryAfter(tc.value, time.Now())
		if !ok || duration != tc.want {
			t.Fatalf("Retry-After %q = %s, %t; want %s, true", tc.value[:min(len(tc.value), 32)], duration, ok, tc.want)
		}
	}
}

func TestEastmoneySourceIdentityPreservesBytesAndRejectsBlankValues(t *testing.T) {
	for _, tc := range []struct {
		raw  json.RawMessage
		want string
		ok   bool
	}{
		{[]byte(`"a "`), "a ", true},
		{[]byte(`123`), "123", true},
		{[]byte(`""`), "", false},
		{[]byte(`"  \t"`), "", false},
		{[]byte(`{`), "", false},
	} {
		got, err := rawJSONSourceID(tc.raw)
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("rawJSONSourceID(%s) = %q, %v; want %q, ok=%t", tc.raw, got, err, tc.want, tc.ok)
		}
	}
	if code, ok := eastmoneyResponseCode([]byte(`9201`)); !ok || code != 9201 {
		t.Fatalf("response code = %d, %t", code, ok)
	}
	if _, ok := eastmoneyResponseCode([]byte(`"9201"`)); ok {
		t.Fatal("string response code unexpectedly accepted")
	}
}

func TestEastmoneyCorporateActionIDIsStableAcrossPayloadCorrections(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	before := []byte(`{"success":true,"code":0,"result":{"count":1,"pages":1,"pageNumber":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.20","BONUS_RATIO":"1.50","IT_RATIO":"0.50"}]}}`)
	after := []byte(`{"success":true,"code":0,"result":{"count":1,"pages":1,"pageNumber":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.50","BONUS_RATIO":"2.00","IT_RATIO":"0.25"}]}}`)
	oldActions, err := ParseEastmoneyCorporateActions(before, id)
	if err != nil {
		t.Fatalf("parse before: %v", err)
	}
	newActions, err := ParseEastmoneyCorporateActions(after, id)
	if err != nil {
		t.Fatalf("parse after: %v", err)
	}
	if len(oldActions) != 2 || len(newActions) != 2 || oldActions[0].ID != newActions[0].ID || oldActions[1].ID != newActions[1].ID {
		t.Fatalf("corrected payload changed source IDs: before=%+v after=%+v", oldActions, newActions)
	}
}

func TestEastmoneyCorporateActionsRejectAmbiguousSameKindSameDay(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	body := []byte(`{"success":true,"code":0,"result":{"count":2,"pages":1,"pageNumber":1,"data":[{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.20","BONUS_RATIO":"0","IT_RATIO":"0"},{"SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.50","BONUS_RATIO":"0","IT_RATIO":"0"}]}}`)
	actions, err := ParseEastmoneyCorporateActions(body, id)
	if !errors.Is(err, ErrIncompleteData) || actions != nil {
		t.Fatalf("actions=%v err=%v, want atomic ErrIncompleteData", actions, err)
	}
}

func TestEastmoneyCorporateActionsUseUpstreamRecordIDWhenAvailable(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	body := []byte(`{"success":true,"code":0,"result":{"count":2,"pages":1,"pageNumber":1,"data":[{"ID":"record-a","SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.20","BONUS_RATIO":"0","IT_RATIO":"0"},{"ID":"record-b","SECURITY_CODE":"600000","EX_DIVIDEND_DATE":"2024-06-13","PRETAX_BONUS_RMB":"4.20","BONUS_RATIO":"0","IT_RATIO":"0"}]}}`)
	actions, err := ParseEastmoneyCorporateActions(body, id)
	if err != nil || len(actions) != 2 || actions[0].ID == actions[1].ID {
		t.Fatalf("upstream record IDs were not preserved: actions=%+v err=%v", actions, err)
	}
}

func TestEastmoneyCorporateActionPaginationRequiresCompleteMetadataAndRows(t *testing.T) {
	id := eastmoneyInstrument(t, market.SSE, "600000")
	for _, tc := range []struct {
		name string
		body string
		want error
	}{
		{"null data", `{"success":true,"code":0,"result":{"count":0,"pages":0,"pageNumber":1,"data":null}}`, ErrMalformedResponse},
		{"empty nonempty page", `{"success":true,"code":0,"result":{"count":1,"pages":1,"pageNumber":1,"data":[]}}`, ErrIncompleteData},
		{"wrong page count", `{"success":true,"code":0,"result":{"count":501,"pages":1,"pageNumber":1,"data":[]}}`, ErrIncompleteData},
		{"zero pages with data", `{"success":true,"code":0,"result":{"count":0,"pages":0,"pageNumber":1,"data":[{}]}}`, ErrIncompleteData},
		{"missing count", `{"success":true,"code":0,"result":{"pages":0,"pageNumber":1,"data":[]}}`, ErrMalformedResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actions, err := ParseEastmoneyCorporateActions([]byte(tc.body), id)
			if !errors.Is(err, tc.want) || actions != nil {
				t.Fatalf("actions=%v err=%v, want atomic %v", actions, err, tc.want)
			}
		})
	}
}

func TestUpstreamErrorRedactsCauseButKeepsUnwrap(t *testing.T) {
	secret := "https://example.test/path?token=secret"
	cause := &url.Error{Op: "Get", URL: secret, Err: context.DeadlineExceeded}
	err := upstreamError(ErrUpstreamTimeout, cause, http.StatusGatewayTimeout, "3")
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "example.test") {
		t.Fatalf("unsafe upstream error text: %q", err)
	}
	if !errors.Is(err, ErrUpstreamTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unwrap lost error identity: %v", err)
	}
	if unsafe := upstreamError(errors.New("kind token=secret"), cause, 0, ""); strings.Contains(unsafe.Error(), "secret") {
		t.Fatalf("unsafe custom kind text: %q", unsafe)
	}
}

func TestEastmoneyClientDefaultsWithoutMutatingInjectedClient(t *testing.T) {
	injected := &http.Client{}
	source := NewEastmoneyMarketSourceWithClient(injected, "", "")
	if injected.Timeout != 0 || source.client == injected || source.client.Timeout != eastmoneyHTTPTimeout {
		t.Fatalf("injected=%+v source=%+v", injected, source.client)
	}
	positive := &http.Client{Timeout: time.Second}
	withPositive := NewEastmoneyMarketSourceWithClient(positive, "", "")
	if positive.Timeout != time.Second || withPositive.client.Timeout != time.Second {
		t.Fatalf("positive timeout was not preserved: injected=%s source=%s", positive.Timeout, withPositive.client.Timeout)
	}
}

func TestEastmoneyRequestPreservesBaseQueryRejectsFragmentsAndClassifiesPastDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nested/kline" || r.URL.Query().Get("fixed") != "yes" {
			t.Fatalf("request path/query = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("fqt") == "0" {
			_, _ = w.Write(readEastmoneyFixture(t, "eastmoney_kline_raw.json"))
			return
		}
		_, _ = w.Write(readEastmoneyFixture(t, "eastmoney_kline_qfq.json"))
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL+"/nested/kline?fixed=yes", srv.URL)
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if err != nil {
		t.Fatalf("base query request: %v", err)
	}
	fragment := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: time.Second}, srv.URL+"/nested/kline#fragment", srv.URL)
	_, _, err = fragment.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("fragment error = %v, want ErrInvalidRequest", err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, _, err = source.FetchBars(ctx, eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, ErrUpstreamTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("past deadline error = %v", err)
	}
}

func TestEastmoneyTimeoutAlsoBoundsResponseBodyRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	source := NewEastmoneyMarketSourceWithClient(&http.Client{Timeout: 30 * time.Millisecond}, srv.URL, srv.URL)
	_, _, err := source.FetchBars(context.Background(), eastmoneyInstrument(t, market.SSE, "600000"), market.Day, eastmoneyDate(2), eastmoneyDate(3))
	if !errors.Is(err, ErrUpstreamTimeout) {
		t.Fatalf("body-read timeout error = %v", err)
	}
}

func TestParseRetryAfterNeverOverflowsNegative(t *testing.T) {
	duration, ok := parseRetryAfter("9223372036854775807", time.Now())
	if !ok || duration < 0 {
		t.Fatalf("overflow Retry-After = %s, %t", duration, ok)
	}
}
