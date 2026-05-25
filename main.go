package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"golang.org/x/time/rate"

	"trading/api"
	"trading/business"
	"trading/config"
	"trading/data"
	"trading/pkg/broker"
)

// brokerRequestInterval 全局 broker 请求最小间隔，避免对新浪接口造成过大压力
const brokerRequestInterval = 3 * time.Second

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	log.Printf("config loaded: db=%s@%s:%d/%s", cfg.DB.User, cfg.DB.Host, cfg.DB.Port, cfg.DB.DBName)

	d, err := data.New(cfg.DB)
	if err != nil {
		log.Fatalf("init data layer failed: %v", err)
	}

	b := broker.NewSinaBroker()
	brokerLimiter := rate.NewLimiter(rate.Every(brokerRequestInterval), 1)
	svc := business.NewStockDataService(b, d.StockKlineDaily(), d.StockKlineWeekly(), brokerLimiter)
	financialSvc := business.NewFinancialReportService(b, d.FinancialReport())

	scheduler := business.NewScheduler(svc, d.StockKlineDaily(), d.StockKlineWeekly())
	scheduler.Start(context.Background(), 16, 0)

	financialScheduler := business.NewFinancialScheduler(financialSvc, d.FinancialReport())
	financialScheduler.Start(context.Background())

	stockInfo, err := business.NewStockInfoProvider(d.StockInfo())
	if err != nil {
		log.Printf("warn: init stock info provider failed: %v", err)
		stockInfo = nil
	}

	signalSvc := business.NewSignalService(d.StockKlineDaily(), d.StockKlineWeekly(), d.FinancialReport(), stockInfo)
	querySvc := business.NewQueryService(d.StockKlineDaily(), d.StockKlineWeekly(), d.FinancialReport())
	macroSvc := business.NewMacroService(broker.NewEastMoneyBroker(), broker.NewSinaBroker())
	r := api.NewRouter(svc, financialSvc, scheduler, financialScheduler, signalSvc, querySvc, macroSvc)

	log.Println("Server starting on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server error: %v", err)
	}
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
