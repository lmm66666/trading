package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
)

const testUpdaterToken = "0123456789abcdef0123456789abcdef"

func TestRemoteRefreshReachesAuthenticatedUpdater(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	ingestion := &refreshIngestionFake{}
	trigger := &refreshTriggerFake{}
	updater, err := NewUpdaterRouter(KernelServices{MarketIngestion: ingestion, Instruments: &refreshLookupFake{ids: []market.InstrumentID{id}}, MarketTrigger: trigger, MarketWorkers: 8}, testUpdaterToken)
	require.NoError(t, err)
	server := httptest.NewServer(updater)
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, testUpdaterToken)
	require.NoError(t, err)
	workbench := NewRouter(KernelServices{RemoteRefresh: client})
	w := refreshRequest(t, workbench, `{"exchange":"SSE","code":"600000"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"version":5`)
	require.Equal(t, id, ingestion.id)
	w = refreshRequest(t, workbench, "")
	require.Equal(t, 202, w.Code, w.Body.String())
	require.Equal(t, 8, trigger.workers)
	for _, path := range []string{"/api/v1/strategies", "/api/v1/market/refresh", "/", "/internal/v1/other"} {
		w := httptest.NewRecorder()
		updater.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 404, w.Code)
	}
}

func TestUpdaterRejectsUnauthorizedRequests(t *testing.T) {
	updater, err := NewUpdaterRouter(KernelServices{}, testUpdaterToken)
	require.NoError(t, err)
	for _, token := range []string{"", "Bearer wrong", "Bearer " + testUpdaterToken + "x"} {
		req := httptest.NewRequest("POST", "/internal/v1/market/refresh", nil)
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		updater.ServeHTTP(w, req)
		require.Equal(t, 401, w.Code)
		require.Contains(t, w.Body.String(), "UNAUTHORIZED")
	}
	_, err = NewUpdaterRouter(KernelServices{}, "")
	require.Error(t, err)
}

func TestUpdaterClientRejectsUnsafeConfiguration(t *testing.T) {
	for _, raw := range []string{"", "ftp://nas", "http://user:pass@nas", "http://nas/path", "http://nas?q=x", "http://nas#x", "http://nas?", "http://nas:bad"} {
		_, err := NewUpdaterClient(raw, testUpdaterToken)
		require.Error(t, err, raw)
	}
	for _, token := range []string{"", "short", testUpdaterToken + "\r\nheader"} {
		_, err := NewUpdaterClient("http://nas:8081", token)
		require.Error(t, err)
	}
}

func TestRemoteRefreshSanitizesUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		want    int
		message string
	}{
		{400, `{"code":400,"message":"INVALID_REQUEST","data":null}`, 400, "INVALID_REQUEST"},
		{404, `{"code":404,"message":"NOT_FOUND","data":null}`, 404, "NOT_FOUND"},
		{409, `{"code":409,"message":"AMBIGUOUS_INSTRUMENT","data":null}`, 409, "AMBIGUOUS_INSTRUMENT"},
		{429, `{"code":429,"message":"MARKET_REFRESH_ALREADY_RUNNING","data":null}`, 429, "MARKET_REFRESH_ALREADY_RUNNING"},
		{401, `secret credentials`, 502, "UPDATER_BAD_RESPONSE"},
		{500, `secret SQL`, 502, "UPDATER_BAD_RESPONSE"},
		{400, `{"code":400,"message":"secret SQL","data":null}`, 502, "UPDATER_BAD_RESPONSE"},
		{200, `{"code":0,"message":"success","data":{}}`, 502, "UPDATER_BAD_RESPONSE"},
		{202, `{"code":0,"message":"success","data":{"status":"bad"}}`, 502, "UPDATER_BAD_RESPONSE"},
		{202, `{"code":0,"message":"success","data":{"status":"ACCEPTED"}} {}`, 502, "UPDATER_BAD_RESPONSE"},
		{202, strings.Repeat(" ", 1<<20) + `{}`, 502, "UPDATER_BAD_RESPONSE"},
	} {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.body[:min(len(tc.body), 40)]), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			client, err := NewUpdaterClient(server.URL, testUpdaterToken)
			require.NoError(t, err)
			w := refreshRequest(t, NewRouter(KernelServices{RemoteRefresh: client}), "")
			require.Equal(t, tc.want, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.message)
			require.NotContains(t, w.Body.String(), "secret")
		})
	}
}

func TestRemoteRefreshDoesNotRetryOrFollowRedirects(t *testing.T) {
	var calls, redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Redirect(w, r, destination.URL, 307) }))
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, testUpdaterToken)
	require.NoError(t, err)
	w := refreshRequest(t, NewRouter(KernelServices{RemoteRefresh: client}), "")
	require.Equal(t, 502, w.Code)
	require.Equal(t, int32(1), calls.Load())
	require.Zero(t, redirected.Load())
	server.Close()
	w = refreshRequest(t, NewRouter(KernelServices{RemoteRefresh: client}), "")
	require.Equal(t, 503, w.Code)
}

func TestRemoteRefreshTimeoutAndCancellation(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, testUpdaterToken)
	require.NoError(t, err)
	client.http.Timeout = 20 * time.Millisecond
	w := refreshRequest(t, NewRouter(KernelServices{RemoteRefresh: client}), "")
	require.Equal(t, 504, w.Code)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("request cancellation not propagated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = client.refresh(ctx, marketRefreshRequest{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRemoteRefreshValidatesBeforeSending(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, testUpdaterToken)
	require.NoError(t, err)
	for _, body := range []string{`{"code":"600000"}`, `{"exchange":"SSE","code":"bad"}`, `{"extra":true}`, strings.Repeat(" ", (1<<20)+1)} {
		w := refreshRequest(t, NewRouter(KernelServices{RemoteRefresh: client}), body)
		require.Equal(t, 400, w.Code)
	}
	require.Zero(t, calls.Load())
}

func TestRemoteRefreshRejectsWrongOrLeakingSuccessPayload(t *testing.T) {
	for _, data := range []string{
		`{"instrument":{"exchange":"SSE","code":"600000"},"version":5,"quality":"secret SQL","daily_bars":10,"weekly_bars":2}`,
		`{"instrument":{"exchange":"SSE","code":"600001"},"version":5,"quality":"COMPLETE","daily_bars":10,"weekly_bars":2}`,
		`{"instrument":{"exchange":"SSE","code":"600000"},"version":0,"quality":"COMPLETE","daily_bars":10,"weekly_bars":2}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"code":0,"message":"success","data":%s}`, data)
		}))
		client, err := NewUpdaterClient(server.URL, testUpdaterToken)
		require.NoError(t, err)
		w := refreshRequest(t, NewRouter(KernelServices{RemoteRefresh: client}), `{"exchange":"SSE","code":"600000"}`)
		server.Close()
		require.Equal(t, 502, w.Code)
		require.NotContains(t, w.Body.String(), "secret")
	}
}

// writeFailureConn simulates a cached connection failing before the next POST
// writes any bytes. With GetBody set, Go would silently redial and replay it.
type writeFailureConn struct {
	net.Conn
	fail *atomic.Bool
}

func (c *writeFailureConn) Write(body []byte) (int, error) {
	if c.fail.Load() {
		return 0, io.ErrClosedPipe
	}
	return c.Conn.Write(body)
}

func TestRemoteRefreshDoesNotReplayOnReusedConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(202)
		fmt.Fprint(w, `{"code":0,"message":"success","data":{"status":"ACCEPTED"}}`)
	}))
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, testUpdaterToken)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if client.http.Transport != nil {
		transport = client.http.Transport.(*http.Transport)
	}
	var dials atomic.Int32
	var fail atomic.Bool
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if dials.Add(1) == 1 {
			return &writeFailureConn{Conn: conn, fail: &fail}, nil
		}
		return conn, nil
	}
	client.http.Transport = transport
	defer transport.CloseIdleConnections()
	_, _, err = client.refresh(context.Background(), marketRefreshRequest{})
	require.NoError(t, err)
	fail.Store(true)
	_, _, err = client.refresh(context.Background(), marketRefreshRequest{})
	require.Error(t, err, "a broken POST must not be transparently replayed")
	require.Equal(t, int32(1), dials.Load(), "no replacement connection should be opened for a retry")
}

func TestRemoteRefreshUsesHTTP1EvenWhenServerOffersHTTP2(t *testing.T) {
	var protocol atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocol.Store(int32(r.ProtoMajor))
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(202)
		fmt.Fprint(w, `{"code":0,"message":"success","data":{"status":"ACCEPTED"}}`)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, testUpdaterToken)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if client.http.Transport != nil {
		transport = client.http.Transport.(*http.Transport)
	}
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	client.http.Transport = transport
	defer transport.CloseIdleConnections()
	_, _, err = client.refresh(context.Background(), marketRefreshRequest{})
	require.NoError(t, err)
	require.Equal(t, int32(1), protocol.Load(), "HTTP/2 may replay POST after protocol errors")
}
