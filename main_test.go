package main

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"trading/api"
)

func TestKernelCompositionKeepsDurableIdempotencyReader(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	kernel, pool, err := newKernel(db)
	require.NoError(t, err)
	require.NotNil(t, pool)
	router := api.NewRouter(nil, nil, nil, nil, nil, nil, nil, kernel)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/strategies", nil))
	require.Equal(t, 200, w.Code)
	mock.ExpectQuery("SELECT .*t_compute_runs").WillReturnError(errors.New("database secret"))
	request := `{"instrument":"SSE:600000","strategy":"daily_b1_buy","strategy_version":"1","idempotency_key":"abc","start":"2026-01-01T00:00:00Z","end":"2026-06-01T00:00:00Z","config":{"initial_cash":1000000000,"cash_fraction_bps":10000,"lot_size":100}}`
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/backtest-runs", strings.NewReader(request)))
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "secret")
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestLoadConfigAndStartupValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("Config:\n  DB:\n    Host: localhost\n"), 0600))
	_, err := loadConfig(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("["), 0600))
	_, err = loadConfig(path)
	require.Error(t, err)
	require.Error(t, run(context.Background(), filepath.Join(dir, "missing")))
}

func TestServerWorkerFailureStopsHTTPAndJoinsWorker(t *testing.T) {
	failure := errors.New("worker failed")
	finished := make(chan struct{})
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	err := serve(context.Background(), server, func(context.Context) error { defer close(finished); return failure })
	require.ErrorIs(t, err, failure)
	select {
	case <-finished:
	default:
		t.Fatal("worker leaked")
	}
}
func TestServerStartupFailureCancelsWorker(t *testing.T) {
	finished := make(chan struct{})
	server := &http.Server{Addr: "invalid::address"}
	err := serve(context.Background(), server, func(ctx context.Context) error { defer close(finished); <-ctx.Done(); return ctx.Err() })
	require.Error(t, err)
	select {
	case <-finished:
	default:
		t.Fatal("worker leaked")
	}
}
func TestServerShutdownWaitsForWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	server := &http.Server{Addr: "127.0.0.1:0"}
	go func() {
		done <- serve(ctx, server, func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() })
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked")
	}
}

func TestServerCloseDoesNotHideWorkerFailureDuringShutdown(t *testing.T) {
	server := &http.Server{Addr: "127.0.0.1:0"}
	require.NoError(t, server.Close())
	failure := errors.New("worker persistence failed")
	err := serve(context.Background(), server, func(ctx context.Context) error { <-ctx.Done(); return failure })
	require.ErrorIs(t, err, failure)
}
