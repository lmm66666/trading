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
	"trading/config"
)

func TestKernelCompositionKeepsDurableIdempotencyReader(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	kernel, err := newKernel(context.Background(), db, config.WorkerConfig{}, config.MarketConfig{})
	require.NoError(t, err)
	require.NotNil(t, kernel.workers)
	require.NotNil(t, kernel.marketScheduler)
	require.Nil(t, kernel.futuresScheduler)
	router := api.NewRouter(kernel.services)
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
	require.NoError(t, os.WriteFile(path, []byte("Config:\n  DB:\n    Host: localhost\n  Worker:\n    Count: 6\n    LeaseSeconds: 45\n    PollIntervalMillis: 500\n    SyncWaitTimeoutSecs: 3\n    ScanBatchSize: 12\n  Market:\n    StockRequestIntervalSeconds: 7\n    FuturesEnabled: true\n    FuturesRefreshIntervalHours: 12\n"), 0600))
	cfg, err := loadConfig(path)
	require.NoError(t, err)
	require.Equal(t, 6, cfg.Worker.Count)
	require.Equal(t, 45, cfg.Worker.LeaseSeconds)
	require.Equal(t, 500, cfg.Worker.PollIntervalMillis)
	require.Equal(t, 3, cfg.Worker.SyncWaitTimeoutSecs)
	require.Equal(t, 12, cfg.Worker.ScanBatchSize)
	require.Equal(t, 7, cfg.Market.StockRequestIntervalSeconds)
	require.True(t, cfg.Market.FuturesEnabled)
	require.Equal(t, 12, cfg.Market.FuturesRefreshIntervalHours)
	require.NoError(t, os.WriteFile(path, []byte("["), 0600))
	_, err = loadConfig(path)
	require.Error(t, err)
	require.Error(t, run(context.Background(), filepath.Join(dir, "missing")))
}

func TestResolveMarketConfigDefaultsAndRejectsUnsafeRate(t *testing.T) {
	got, err := resolveMarketConfig(config.MarketConfig{})
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, got.StockRequestInterval)

	got, err = resolveMarketConfig(config.MarketConfig{StockRequestIntervalSeconds: 10})
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, got.StockRequestInterval)
	require.False(t, got.FuturesEnabled)

	got, err = resolveMarketConfig(config.MarketConfig{FuturesEnabled: true})
	require.NoError(t, err)
	require.True(t, got.FuturesEnabled)
	require.Equal(t, 24*time.Hour, got.FuturesRefreshInterval)

	got, err = resolveMarketConfig(config.MarketConfig{FuturesEnabled: true, FuturesRefreshIntervalHours: 12})
	require.NoError(t, err)
	require.Equal(t, 12*time.Hour, got.FuturesRefreshInterval)

	_, err = resolveMarketConfig(config.MarketConfig{StockRequestIntervalSeconds: 4})
	require.Error(t, err)
	_, err = resolveMarketConfig(config.MarketConfig{FuturesEnabled: true, FuturesRefreshIntervalHours: 169})
	require.Error(t, err)
}

func TestKernelCompositionWiresOptionalFuturesScheduler(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	kernel, err := newKernel(context.Background(), db, config.WorkerConfig{}, config.MarketConfig{FuturesEnabled: true})
	require.NoError(t, err)
	require.NotNil(t, kernel.futuresScheduler)
}

func TestResolveWorkerConfigDefaultsAndRejectsOutOfBounds(t *testing.T) {
	got, err := resolveWorkerConfig(config.WorkerConfig{})
	require.NoError(t, err)
	require.Equal(t, 4, got.Count)
	require.Equal(t, 30*time.Second, got.Lease)
	require.Equal(t, 250*time.Millisecond, got.PollInterval)
	require.Equal(t, 2*time.Second, got.SyncWaitTimeout)
	require.Equal(t, 8, got.ScanBatchSize)

	_, err = resolveWorkerConfig(config.WorkerConfig{Count: 65})
	require.Error(t, err)
	_, err = resolveWorkerConfig(config.WorkerConfig{Count: 1, LeaseSeconds: 1, PollIntervalMillis: 1, SyncWaitTimeoutSecs: 1, ScanBatchSize: 65})
	require.Error(t, err)
}

func TestRunBackgroundValidatesAllRunnersBeforeStarting(t *testing.T) {
	started := make(chan struct{})
	err := runBackground(context.Background(), func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	require.Error(t, err)
	select {
	case <-started:
		t.Fatal("background service started before configuration validation completed")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRunBackgroundCancelsAndJoinsPeersOnFailure(t *testing.T) {
	failure := errors.New("scheduler failed")
	joined := make(chan struct{})
	err := runBackground(context.Background(),
		func(context.Context) error { return failure },
		func(ctx context.Context) error {
			defer close(joined)
			<-ctx.Done()
			return ctx.Err()
		},
	)
	require.ErrorIs(t, err, failure)
	select {
	case <-joined:
	default:
		t.Fatal("peer background service was not joined")
	}
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
