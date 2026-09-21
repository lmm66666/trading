package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goccy/go-yaml"
	"gorm.io/gorm"

	"trading/api"
	"trading/config"
	"trading/data"
	"trading/internal/application"
	"trading/internal/backtest"
	mysqlinfra "trading/internal/infrastructure/mysql"
	"trading/internal/port"
	"trading/internal/strategy"
	"trading/internal/strategy/builtin"
)

const marketRefreshInterval = 24 * time.Hour

type workerRuntimeConfig struct {
	Count           int
	Lease           time.Duration
	PollInterval    time.Duration
	SyncWaitTimeout time.Duration
	ScanBatchSize   int
}

type marketRuntimeConfig struct {
	StockRequestInterval   time.Duration
	FuturesEnabled         bool
	FuturesRefreshInterval time.Duration
}

type kernelRuntime struct {
	progress               *application.RefreshProgress
	services               api.KernelServices
	workers                *application.WorkerPool
	marketScheduler        *application.MarketScheduler
	futuresScheduler       *application.FuturesScheduler
	futuresRefreshInterval time.Duration
}

type rootMarketTrigger struct {
	ctx       context.Context
	scheduler *application.MarketScheduler
	futures   *application.FuturesScheduler
}

func (t rootMarketTrigger) TriggerNow(workers int) (port.BatchRefreshReceipt, error) {
	return application.TriggerMarketRefresh(t.ctx, t.scheduler, t.futures, workers)
}

func main() {
	service := flag.String("service", "", "required service: updater or workbench")
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *configPath, *service); err != nil {
		slog.Error("application stopped with an internal error")
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath, service string) error {
	if service != "updater" && service != "workbench" {
		return errors.New("service must be updater or workbench")
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	settings, err := resolveServiceConfig(service, cfg)
	if err != nil {
		return err
	}
	connect := data.Open
	if service == "updater" {
		connect = data.New
	}
	d, err := connect(cfg.DB)
	if err != nil {
		return err
	}
	sqlDB, err := d.DB().DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	kernel, err := newServiceKernel(ctx, d.DB(), cfg, settings)
	if err != nil {
		return err
	}
	var handler http.Handler
	if service == "updater" {
		recoveryBefore := time.Now().UTC().Truncate(time.Microsecond)
		kernel.progress.SetRecovery(func(ctx context.Context) error {
			return mysqlinfra.NewRefreshProgressStore(d.DB()).InterruptRefreshRunsBefore(ctx, recoveryBefore)
		})
		recoveryCtx, stopRecovery := context.WithTimeout(ctx, 2*time.Second)
		kernel.progress.Flush(recoveryCtx)
		stopRecovery()

		router, err := api.NewUpdaterRouter(kernel.services, cfg.Updater.Token)
		if err != nil {
			return err
		}
		handler = router
	} else {
		router := api.NewRouter(kernel.services)
		if err := api.AttachWebUI(router, "web/dist"); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			slog.Warn("Web UI is not built; API-only mode")
		}
		handler = router
	}
	slog.Info("service starting", "service", service, "address", settings.address)
	server := &http.Server{Addr: settings.address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	err = serve(ctx, server, func(ctx context.Context) error {
		var runners []func(context.Context) error
		if kernel.progress != nil {
			runners = append(runners, kernel.progress.Run)
		}
		if kernel.workers != nil {
			runners = append(runners, kernel.workers.Run)
		}
		if kernel.marketScheduler != nil {
			runners = append(runners, func(ctx context.Context) error {
				return kernel.marketScheduler.Start(ctx, marketRefreshInterval, kernel.services.MarketWorkers)
			})
		}
		if kernel.futuresScheduler != nil {
			runners = append(runners, func(ctx context.Context) error {
				return kernel.futuresScheduler.Start(ctx, kernel.futuresRefreshInterval)
			})
		}
		return runBackground(ctx, runners...)
	})
	cancel()
	if kernel.marketScheduler != nil {
		kernel.marketScheduler.Wait()
	}
	if kernel.futuresScheduler != nil {
		kernel.futuresScheduler.Wait()
	}
	if kernel.progress != nil {
		flushCtx, stopFlush := context.WithTimeout(context.Background(), 5*time.Second)
		kernel.progress.Flush(flushCtx)
		stopFlush()
	}
	return err
}

func newServiceKernel(rootCtx context.Context, db *gorm.DB, cfg *config.Config, service serviceSettings) (kernelRuntime, error) {
	if service.role == "updater" {
		return newUpdaterKernel(rootCtx, db, cfg)
	}
	settings, err := resolveWorkerConfig(cfg.Worker)
	if err != nil {
		return kernelRuntime{}, err
	}
	registry := &strategy.Registry{}
	if err := builtin.RegisterAll(registry); err != nil {
		return kernelRuntime{}, err
	}
	marketData := mysqlinfra.NewMarketDataRepository(db)
	instrumentQueries, err := application.NewInstrumentQueryService(marketData)
	if err != nil {
		return kernelRuntime{}, err
	}
	chartQueries, err := application.NewChartQueryService(marketData, marketData)
	if err != nil {
		return kernelRuntime{}, err
	}
	watchlistStore := mysqlinfra.NewWatchlistStore(db)
	watchlist, err := application.NewWatchlistService(watchlistStore, watchlistStore, marketData)
	if err != nil {
		return kernelRuntime{}, err
	}
	chartBoards, err := application.NewChartBoardService(mysqlinfra.NewChartBoardStore(db))
	if err != nil {
		return kernelRuntime{}, err
	}
	queue := mysqlinfra.NewJobQueue(db)
	store := mysqlinfra.NewRunStore(db)
	snapshots := mysqlinfra.NewSignalSnapshotStore(db)
	telemetry := application.SlogTelemetry{Logger: slog.Default()}
	compute := application.ComputeConfig{EngineVersion: "strategy-kernel-v1", Telemetry: telemetry}
	backtests, err := application.NewBacktestService(registry, backtest.Engine{}, marketData, queue, store, compute)
	if err != nil {
		return kernelRuntime{}, err
	}
	scans, err := application.NewScanService(registry, marketData, queue, store, snapshots, application.ScanConfig{ComputeConfig: compute, Workers: settings.ScanBatchSize})
	if err != nil {
		return kernelRuntime{}, err
	}
	workers, err := application.NewWorkerPool(queue, store, map[port.RunKind]application.RunHandler{port.RunBacktest: backtests.Execute, port.RunScan: scans.Execute}, application.WorkerPoolConfig{Owner: fmt.Sprintf("trading-worker-%d", os.Getpid()), Workers: settings.Count, Lease: settings.Lease, PollInterval: settings.PollInterval, Telemetry: telemetry})
	if err != nil {
		return kernelRuntime{}, err
	}
	services := api.KernelServices{
		Backtests:         backtests,
		Scans:             scans,
		Runs:              store,
		Registry:          registry,
		Instruments:       marketData,
		SnapshotKeys:      snapshots,
		RemoteRefresh:     service.client,
		RefreshQueries:    application.NewRefreshQueries(mysqlinfra.NewRefreshProgressStore(db)),
		MarketQueries:     application.NewMarketQueryService(marketData),
		InstrumentCatalog: instrumentQueries,
		ChartQueries:      chartQueries,
		Watchlist:         watchlist,
		ChartBoards:       chartBoards,
		MarketWorkers:     settings.ScanBatchSize,
		SyncWaitTimeout:   settings.SyncWaitTimeout,
		PollInterval:      settings.PollInterval,
		Clock:             time.Now,
	}
	return kernelRuntime{services: services, workers: workers}, nil
}

func resolveMarketConfig(input config.MarketConfig) (marketRuntimeConfig, error) {
	if input.StockRequestIntervalSeconds == 0 {
		input.StockRequestIntervalSeconds = 5
	}
	if input.StockRequestIntervalSeconds < 5 || input.StockRequestIntervalSeconds > 86_400 {
		return marketRuntimeConfig{}, errors.New("market configuration is out of bounds")
	}
	if input.FuturesRefreshIntervalHours == 0 {
		input.FuturesRefreshIntervalHours = 24
	}
	if input.FuturesRefreshIntervalHours < 1 || input.FuturesRefreshIntervalHours > 168 {
		return marketRuntimeConfig{}, errors.New("market configuration is out of bounds")
	}
	return marketRuntimeConfig{
		StockRequestInterval:   time.Duration(input.StockRequestIntervalSeconds) * time.Second,
		FuturesEnabled:         input.FuturesEnabled,
		FuturesRefreshInterval: time.Duration(input.FuturesRefreshIntervalHours) * time.Hour,
	}, nil
}

func resolveWorkerConfig(input config.WorkerConfig) (workerRuntimeConfig, error) {
	if input.Count <= 0 {
		input.Count = 4
	}
	if input.LeaseSeconds <= 0 {
		input.LeaseSeconds = 30
	}
	if input.PollIntervalMillis <= 0 {
		input.PollIntervalMillis = 250
	}
	if input.SyncWaitTimeoutSecs <= 0 {
		input.SyncWaitTimeoutSecs = 2
	}
	if input.ScanBatchSize <= 0 {
		input.ScanBatchSize = 8
	}
	if input.Count > 64 || input.ScanBatchSize > application.MaxMarketWorkers || input.LeaseSeconds > 86_400 || input.PollIntervalMillis > 60_000 || input.SyncWaitTimeoutSecs > 300 {
		return workerRuntimeConfig{}, errors.New("worker configuration is out of bounds")
	}
	return workerRuntimeConfig{
		Count:           input.Count,
		Lease:           time.Duration(input.LeaseSeconds) * time.Second,
		PollInterval:    time.Duration(input.PollIntervalMillis) * time.Millisecond,
		SyncWaitTimeout: time.Duration(input.SyncWaitTimeoutSecs) * time.Second,
		ScanBatchSize:   input.ScanBatchSize,
	}, nil
}

// runBackground owns all long-running services. The first exit cancels and
// joins the others so no goroutine can use the database after run returns.
func runBackground(ctx context.Context, runners ...func(context.Context) error) error {
	if len(runners) == 0 {
		return errors.New("no background services configured")
	}
	for _, runner := range runners {
		if runner == nil {
			return errors.New("nil background service")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, len(runners))
	for _, runner := range runners {
		go func(run func(context.Context) error) { done <- run(ctx) }(runner)
	}
	result := <-done
	cancel()
	for remaining := len(runners) - 1; remaining > 0; remaining-- {
		err := <-done
		if result == nil || errors.Is(result, context.Canceled) {
			if err != nil && !errors.Is(err, context.Canceled) {
				result = err
			}
		}
	}
	return result
}

// serve 共同管理 HTTP 与持久化 worker；任一失败取消根 context，并在返回
// 前等待全部退出，使调用方随后关闭数据库时没有仍在运行的任务。
func serve(ctx context.Context, server *http.Server, worker func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server.BaseContext = func(net.Listener) context.Context { return ctx }
	httpDone := make(chan error, 1)
	workerDone := make(chan error, 1)
	go func() { httpDone <- server.ListenAndServe() }()
	go func() { workerDone <- worker(ctx) }()
	var result error
	var httpFinished, workerFinished bool
	select {
	case <-ctx.Done():
	case result = <-httpDone:
		httpFinished = true
	case result = <-workerDone:
		workerFinished = true
	}
	// 正常关闭信号不能掩盖随后收集到的 worker/HTTP 实际故障。
	if errors.Is(result, context.Canceled) || errors.Is(result, http.ErrServerClosed) {
		result = nil
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	shutdownErr := server.Shutdown(shutdownCtx)
	stop()
	if shutdownErr != nil {
		_ = server.Close()
		if result == nil {
			result = shutdownErr
		}
	}
	if !httpFinished {
		err := <-httpDone
		if result == nil && !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	}
	if !workerFinished {
		err := <-workerDone
		if result == nil && !errors.Is(err, context.Canceled) {
			result = err
		}
	}
	return result
}

func loadConfig(configPath string) (*config.Config, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Config config.Config `yaml:"Config"`
	}
	if err := yaml.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Config, nil
}
