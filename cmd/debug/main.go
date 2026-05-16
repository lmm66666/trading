package main

import (
	"context"
	"fmt"
	"os"

	"trading/config"
	"trading/data"
	"trading/model"
	"trading/pkg/filter"
	"trading/pkg/indicator"
	"trading/pkg/strategy"

	"gopkg.in/yaml.v3"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg := loadConfig(cfgPath)

	d, err := data.New(cfg.DB)
	if err != nil {
		fmt.Println("data.New error:", err)
		return
	}
	repo := d.StockKlineDaily()

	ctx := context.Background()
	dailies, err := repo.FindByCode(ctx, "600150", 70)
	if err != nil || len(dailies) == 0 {
		fmt.Println("no data for 600150:", err)
		return
	}

	klines := make([]*model.StockKline, len(dailies))
	for i, d := range dailies {
		klines[i] = &model.StockKline{
			Code: d.Code, Date: d.Date,
			Open: d.Open, High: d.High, Low: d.Low, Close: d.Close, Volume: d.Volume,
		}
	}

	fmt.Printf("=== 600150 最近 %d 条 ===\n", len(klines))
	for i := 0; i < len(klines); i++ {
		k := klines[i]
		fmt.Printf("%s O=%.2f H=%.2f L=%.2f C=%.2f V=%d\n", k.Date, k.Open, k.High, k.Low, k.Close, k.Volume)
	}

	// 逐个 filter
	cfg_vs := filter.VolumeSurgeConfig{
		VolumeMAPeriod: 20, MinVolumeRatio: 2.0, MinRallyPct: 5.0,
		MaxPullbackPct: 15.0, MaxPullbackDays: 10, MaxPullbackVolRatio: 0,
		NearLowPeriod: 60, NearLowMaxRatio: 0.15,
		SurgeWindowDays: 3, MaxPullbackToVMARatio: 1.5,
	}

	filters := []struct {
		name   string
		filter filter.Filter
	}{
		{"VolumeSurge(含底部确认+窗口合并)", filter.NewVolumeSurgeFilter(cfg_vs)},
		{"VolumeSurge(不含底部确认)", filter.NewVolumeSurgeFilter(filter.VolumeSurgeConfig{
			VolumeMAPeriod: 20, MinVolumeRatio: 2.0, MinRallyPct: 5.0,
			MaxPullbackPct: 15.0, MaxPullbackDays: 10, MaxPullbackVolRatio: 0.5,
		})},
		{"SupportHold(MA20)", filter.NewSupportHoldFilter(20)},
		{"KDJRange(5,40)", filter.NewKDJRangeFilter(5, 40)},
	}

	for _, f := range filters {
		results := f.filter.Filter(klines)
		validDays := 0
		lastValid := ""
		for i, r := range results {
			if r.Valid {
				validDays++
				lastValid = klines[i].Date
			}
		}
		fmt.Printf("\n[%s] 有效天数: %d, 最后有效日: %s\n", f.name, validDays, lastValid)
		for i := len(results) - 5; i < len(results); i++ {
			fmt.Printf("  %s Valid=%v\n", results[i].Date, results[i].Valid)
		}
	}

	// MA20 & KDJ 详情
	prices := make([]float64, len(klines))
	for i, k := range klines {
		prices[i] = k.Close
	}
	ma20 := indicator.ComputeMA(prices, 20)
	kdjResults := indicator.ComputeKDJ(klines)
	fmt.Printf("\n=== 最后15天 MA20 & KDJ ===\n")
	for i := len(klines) - 15; i < len(klines); i++ {
		k := klines[i]
		fmt.Printf("%s C=%.2f MA20=%.2f K=%.2f D=%.2f J=%.2f\n",
			k.Date, k.Close, ma20[i], kdjResults[i].K, kdjResults[i].D, kdjResults[i].J)
	}

	// 完整策略
	st := strategy.NewBottomSurgePullbackStrategy()
	sigs := st.ScanAll(klines)
	fmt.Printf("\n=== 策略 %s 信号 ===\n", st.Name())
	for _, s := range sigs {
		fmt.Println(s.Date)
	}
	if len(sigs) == 0 {
		fmt.Println("(无信号)")
	}
}

func loadConfig(path string) *config.Config {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read config error:", err)
		os.Exit(1)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Println("parse config error:", err)
		os.Exit(1)
	}
	return &cfg
}
