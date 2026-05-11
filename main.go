package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/goccy/go-yaml"

	"trading/api"
	"trading/business"
	"trading/config"
	"trading/data"
	"trading/pkg/broker"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	fmt.Printf("%+v\n", cfg)

	d, err := data.New(cfg.DB)
	if err != nil {
		log.Fatalf("init data layer failed: %v", err)
	}

	b := broker.NewSinaBroker()
	svc := business.NewStockDataService(b, d.StockKlineDaily(), d.StockKlineWeekly())
	financialSvc := business.NewFinancialReportService(b, d.FinancialReport())

	scheduler := business.NewScheduler(svc, d.StockKlineDaily(), d.StockKlineWeekly())
	scheduler.Start(context.Background(), 16, 0)

	financialScheduler := business.NewFinancialScheduler(financialSvc, d.FinancialReport())

	signalSvc := business.NewSignalService(d.StockKlineDaily(), d.StockKlineWeekly(), d.FinancialReport())
	querySvc := business.NewQueryService(d.StockKlineDaily(), d.StockKlineWeekly(), d.FinancialReport())
	r := api.NewRouter(svc, financialSvc, scheduler, financialScheduler, signalSvc, querySvc)

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
