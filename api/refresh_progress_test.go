package api

import (
	"bytes"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"trading/internal/application"
	"trading/internal/port"
)

type progressReaderFake struct{ err error }

func (f progressReaderFake) LatestRefreshRun(_ context.Context, kind string) (port.RefreshRun, error) {
	return port.RefreshRun{RunID: "run-1", Kind: kind, State: "RUNNING"}, f.err
}

func (f progressReaderFake) ListRefreshRuns(_ context.Context, kind string, _ uint64, _ int) ([]port.RefreshRun, error) {
	return []port.RefreshRun{{RunID: "run-1", Kind: kind, State: "RUNNING"}}, f.err
}
func (f progressReaderFake) GetRefreshRun(_ context.Context, id string) (port.RefreshRun, error) {
	if id == "missing" {
		return port.RefreshRun{}, port.ErrRefreshRunNotFound
	}
	return port.RefreshRun{RunID: id}, f.err
}
func (f progressReaderFake) ListRefreshFailures(context.Context, string, uint64, int) ([]port.RefreshFailure, error) {
	return []port.RefreshFailure{}, f.err
}
func TestRefreshProgressQueriesAndAuthentication(t *testing.T) {
	enabled := true
	k := KernelServices{RefreshQueries: application.NewRefreshQueries(progressReaderFake{}), FuturesEnabled: &enabled}
	r := NewRouter(k)
	for _, url := range []string{"/api/v1/market/refresh/status", "/api/v1/market/refresh/runs", "/api/v1/market/refresh/runs/run-1", "/api/v1/market/refresh/runs/run-1/failures"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		require.Equal(t, 200, w.Code, w.Body.String())
	}
	for _, q := range []string{"limit=0", "limit=101", "kind=bad", "limit=1&limit=2", "unknown=1", "before_id=-1"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/market/refresh/runs?"+q, nil))
		require.Equal(t, 400, w.Code, q)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/market/refresh/runs/missing", nil))
	require.Equal(t, 404, w.Code)
	token := strings.Repeat("a", 32)
	internal, err := NewUpdaterRouter(k, token)
	require.NoError(t, err)
	for _, auth := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/internal/v1/market/refresh/status", nil)
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		internal.ServeHTTP(w, req)
		if auth {
			require.Equal(t, http.StatusOK, w.Code)
		} else {
			require.Equal(t, 401, w.Code)
		}
	}
}

func TestRefreshCapabilitiesFailureDoesNotHideDatabaseHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, strings.Repeat("a", 32))
	require.NoError(t, err)
	r := NewRouter(KernelServices{RefreshQueries: application.NewRefreshQueries(progressReaderFake{}), RemoteRefresh: client})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/market/refresh/status", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"futures_enabled":null`)
	require.Contains(t, w.Body.String(), "run-1")
}
func TestUpdaterCapabilitiesValidation(t *testing.T) {
	for _, body := range []string{`{"code":0,"message":"success","data":{"futures_enabled":false}}`, `{"code":0,"message":"success","data":{"futures_enabled":"private"}}`, `{`, `{"code":1,"message":"success","data":{"futures_enabled":true}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/internal/v1/market/refresh/status", r.URL.Path)
			require.NotEmpty(t, r.Header.Get("Authorization"))
			w.Write([]byte(body))
		}))
		client, err := NewUpdaterClient(server.URL, strings.Repeat("a", 32))
		require.NoError(t, err)
		got := client.futuresEnabled(context.Background())
		if strings.Contains(body, ":false") {
			require.NotNil(t, got)
			require.False(t, *got)
		} else {
			require.Nil(t, got)
		}
		server.Close()
	}
}

func TestRefreshQueryErrorsDoNotLeakIntoLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	r := NewRouter(KernelServices{RefreshQueries: application.NewRefreshQueries(progressReaderFake{err: errors.New("password=secret /private/location")})})
	for _, suffix := range []string{"status", "runs", "runs/run-1", "runs/run-1/failures"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/market/refresh/"+suffix, nil))
		require.Equal(t, 500, w.Code)
		require.NotContains(t, w.Body.String(), "secret")
	}
	require.NotContains(t, logs.String(), "secret")
	require.NotContains(t, logs.String(), "/private")
}
