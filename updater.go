package main

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"trading/api"
	"trading/config"
	"trading/internal/application"
	mysqlinfra "trading/internal/infrastructure/mysql"
	"trading/internal/market"
	"trading/internal/port"
	"trading/pkg/broker"
)

func newUpdaterKernel(rootCtx context.Context, db *gorm.DB, cfg *config.Config) (kernelRuntime, error) {
	marketSettings, err := resolveMarketConfig(cfg.Market)
	if err != nil {
		return kernelRuntime{}, err
	}
	workers := cfg.Worker.ScanBatchSize
	if workers <= 0 {
		workers = 8
	}
	marketData := mysqlinfra.NewMarketDataRepository(db)
	historyStart := time.Now().UTC().AddDate(-(port.MaxBacktestRangeYears - 1), 0, 0).Truncate(time.Microsecond)
	sinaLimiter := rate.NewLimiter(rate.Every(marketSettings.StockRequestInterval), 1)
	ingestion, err := application.NewMarketIngestionService(
		broker.NewSinaMarketSource(sinaLimiter),
		marketData,
		marketData,
		application.MarketIngestionConfig{Source: "sina", HistoryStart: historyStart},
	)
	if err != nil {
		return kernelRuntime{}, err
	}
	marketScheduler, err := application.NewMarketScheduler(marketData, ingestion, port.InstrumentScope{Exchanges: []market.Exchange{market.SSE, market.SZSE, market.BSE}, ActiveOnly: true, Limit: port.MaxScanInstruments})
	if err != nil {
		return kernelRuntime{}, err
	}
	progressStore := mysqlinfra.NewRefreshProgressStore(db)
	progress := application.NewRefreshProgress(progressStore)
	marketScheduler.SetProgress(progress)
	var futuresScheduler *application.FuturesScheduler
	if marketSettings.FuturesEnabled {
		futuresIngestion, err := application.NewMarketIngestionService(
			broker.NewSinaFuturesSource(sinaLimiter),
			marketData,
			marketData,
			application.MarketIngestionConfig{Source: "sina-futures", HistoryStart: historyStart, FullHistoryRefresh: true},
		)
		if err != nil {
			return kernelRuntime{}, err
		}
		futuresScheduler, err = application.NewFuturesScheduler(futuresIngestion, application.DefaultSinaFuturesInstruments(), slog.Default())
		if err != nil {
			return kernelRuntime{}, err
		}
	}

	if futuresScheduler != nil {
		futuresScheduler.SetProgress(progress)
	}
	services := api.KernelServices{RefreshQueries: application.NewRefreshQueries(progressStore), FuturesEnabled: &marketSettings.FuturesEnabled, MarketIngestion: ingestion, MarketTrigger: rootMarketTrigger{ctx: rootCtx, scheduler: marketScheduler}, Instruments: marketData, MarketWorkers: workers}
	return kernelRuntime{progress: progress, services: services, marketScheduler: marketScheduler, futuresScheduler: futuresScheduler, futuresRefreshInterval: marketSettings.FuturesRefreshInterval}, nil
}
