package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goccy/go-yaml"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"trading/api"
	"trading/business"
	"trading/config"
	"trading/data"
	"trading/internal/application"
	"trading/internal/backtest"
	mysqlinfra "trading/internal/infrastructure/mysql"
	"trading/internal/port"
	"trading/internal/strategy"
	"trading/internal/strategy/builtin"
	"trading/pkg/broker"
)

// brokerRequestInterval 全局 broker 请求最小间隔，避免对新浪接口造成过大压力
const brokerRequestInterval = 3 * time.Second

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *configPath); err != nil {
		log.Print("application stopped with an internal error")
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	d, err := data.New(cfg.DB)
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
	kernel, workers, err := newKernel(d.DB())
	if err != nil {
		return err
	}

	b := broker.NewSinaBroker()
	brokerLimiter := rate.NewLimiter(rate.Every(brokerRequestInterval), 1)
	svc := business.NewStockDataService(b, d.StockKlineDaily(), d.StockKlineWeekly(), brokerLimiter)
	financialSvc := business.NewFinancialReportService(b, d.FinancialReport())

	scheduler := business.NewScheduler(svc, d.StockKlineDaily(), d.StockKlineWeekly())
	scheduler.Start(ctx, 16, 0)
	defer scheduler.Stop()

	financialScheduler := business.NewFinancialScheduler(financialSvc, d.FinancialReport())
	financialScheduler.Start(ctx)
	defer financialScheduler.Stop()

	// Task15 才迁移财报筛选与旧行情读写。此实例仅服务财报接口，
	// 不注入技术行情仓储，旧 signal/backtest 路径已接入 kernel。
	signalSvc := business.NewSignalService(nil, nil, d.FinancialReport(), nil)
	querySvc := business.NewQueryService(d.StockKlineDaily(), d.StockKlineWeekly(), d.FinancialReport())
	macroSvc := business.NewMacroService(broker.NewEastMoneyBroker(), broker.NewSinaBroker())
	r := api.NewRouter(svc, financialSvc, scheduler, financialScheduler, signalSvc, querySvc, macroSvc, kernel)

	log.Println("Server starting on :8080")
	server := &http.Server{Addr: ":8080", Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	err = serve(ctx, server, workers.Run)
	cancel()
	return err
}

func newKernel(db *gorm.DB) (api.KernelServices, *application.WorkerPool, error) {
	registry := &strategy.Registry{}
	if err := builtin.RegisterAll(registry); err != nil {
		return api.KernelServices{}, nil, err
	}
	marketData := mysqlinfra.NewMarketDataRepository(db)
	queue := mysqlinfra.NewJobQueue(db)
	store := mysqlinfra.NewRunStore(db)
	snapshots := mysqlinfra.NewSignalSnapshotStore(db)
	telemetry := application.SlogTelemetry{Logger: slog.Default()}
	compute := application.ComputeConfig{EngineVersion: "strategy-kernel-v1", Telemetry: telemetry}
	backtests, err := application.NewBacktestService(registry, backtest.Engine{}, marketData, queue, store, compute)
	if err != nil {
		return api.KernelServices{}, nil, err
	}
	scans, err := application.NewScanService(registry, marketData, queue, store, snapshots, application.ScanConfig{ComputeConfig: compute, Workers: 8})
	if err != nil {
		return api.KernelServices{}, nil, err
	}
	workers, err := application.NewWorkerPool(queue, store, map[port.RunKind]application.RunHandler{port.RunBacktest: backtests.Execute, port.RunScan: scans.Execute}, application.WorkerPoolConfig{Owner: "trading-worker", Workers: 4, Lease: 30 * time.Second, PollInterval: 250 * time.Millisecond, Telemetry: telemetry})
	if err != nil {
		return api.KernelServices{}, nil, err
	}
	kernel := api.KernelServices{Backtests: backtests, Scans: scans, Runs: store, Registry: registry, Instruments: marketData, SnapshotKeys: snapshots, SyncWaitTimeout: 2 * time.Second}
	return kernel, workers, nil
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
