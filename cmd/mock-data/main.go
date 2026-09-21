// mock-data 向本地 MySQL 灌入固定种子的演示行情：3 只 A 股与 1 个期货主力
// 连续各 400 根日线及 1/1 复权因子，用于本地联调。生产行情由调度器抓取，
// 不使用本工具；连接配置指向本地测试库。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goccy/go-yaml"
	"gorm.io/gorm"

	"trading/config"
	"trading/data"
	mysqlinfra "trading/internal/infrastructure/mysql"
	"trading/internal/market"
	"trading/internal/port"
)

const (
	mockSource   = "mock"
	mockBarCount = 400
	volumeBase   = int64(1_500_000)
)

type mockInstrument struct {
	id         market.InstrumentID
	name       string
	board      string
	lotSize    int64
	seed       uint32
	startPrice market.Price // 已按 ValueScale 缩放的起始收盘价
}

var mockInstruments = []mockInstrument{
	{id: market.InstrumentID{Exchange: market.SSE, Code: "600519"}, name: "贵州茅台", board: "MAIN", lotSize: 100, seed: 600519, startPrice: scaledYuan(1480)},
	{id: market.InstrumentID{Exchange: market.SZSE, Code: "000858"}, name: "五粮液", board: "MAIN", lotSize: 100, seed: 858, startPrice: scaledYuan(128)},
	{id: market.InstrumentID{Exchange: market.SZSE, Code: "300750"}, name: "宁德时代", board: "MAIN", lotSize: 100, seed: 300750, startPrice: scaledYuan(186)},
	{id: market.InstrumentID{Exchange: market.INE, Code: "SC.MAIN"}, name: "原油主力连续", board: "MAIN", lotSize: 1000, seed: 2024, startPrice: scaledYuan(520)},
}

type publishOutcome struct {
	Instrument market.InstrumentID
	Version    market.DataVersion
	Bars       int
}

type configLoader func(string) (*config.Config, error)

type mockPublisher func(context.Context, *config.Config) ([]publishOutcome, error)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, loadConfig, publishMockData); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer, load configLoader, publish mockPublisher) error {
	flags := flag.NewFlagSet("mock-data", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "config.yaml.local", "本地数据库配置文件")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *configPath == "" {
		return errors.New("参数无效；仅支持 -config")
	}
	cfg, err := load(*configPath)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	outcomes, err := publish(ctx, cfg)
	if err != nil {
		return err
	}
	for _, outcome := range outcomes {
		fmt.Fprintf(out, "%s 已发布 %d 根日线，数据版本 %d\n", outcome.Instrument, outcome.Bars, outcome.Version)
	}
	return nil
}

// loadConfig 与主程序同一配置格式（yaml 顶层 Config 包装）。
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
	cfg := &wrapper.Config
	if cfg.DB.Host == "" || cfg.DB.Port < 1 || cfg.DB.Port > 65535 || cfg.DB.User == "" || cfg.DB.DBName == "" {
		return nil, errors.New("数据库配置不完整")
	}
	return cfg, nil
}

func publishMockData(ctx context.Context, cfg *config.Config) ([]publishOutcome, error) {
	d, err := data.New(cfg.DB)
	if err != nil {
		return nil, errors.New("无法连接本地数据库或完成建表")
	}
	sqlDB, err := d.DB().DB()
	if err != nil {
		return nil, errors.New("无法获取数据库连接")
	}
	defer sqlDB.Close()
	db := d.DB()
	repo := mysqlinfra.NewMarketDataRepository(db)
	anchor := latestTradingDay(time.Now().UTC())
	outcomes := make([]publishOutcome, 0, len(mockInstruments))
	for _, inst := range mockInstruments {
		batch := buildBatch(inst, anchor)
		version, err := repo.Publish(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("发布 %s 演示行情失败", inst.id)
		}
		if err := applyInstrumentMetadata(ctx, db, inst); err != nil {
			return nil, fmt.Errorf("补写 %s 元数据失败", inst.id)
		}
		outcomes = append(outcomes, publishOutcome{Instrument: inst.id, Version: version, Bars: len(batch.Bars[market.Day])})
	}
	return outcomes, nil
}

// applyInstrumentMetadata 补齐 Publish 未写入的展示元数据（名称、板块、活跃与手数）。
func applyInstrumentMetadata(ctx context.Context, db *gorm.DB, inst mockInstrument) error {
	result := db.WithContext(ctx).Model(&mysqlinfra.InstrumentModel{}).
		Where("exchange = ? AND code = ?", string(inst.id.Exchange), inst.id.Code).
		Updates(map[string]any{"name": inst.name, "board": inst.board, "active": true, "lot_size": inst.lotSize})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("instrument %s not found", inst.id)
	}
	return nil
}

func buildBatch(inst mockInstrument, anchor time.Time) port.MarketWriteBatch {
	days := tradingDays(anchor, mockBarCount)
	return port.MarketWriteBatch{
		Source:     mockSource,
		Instrument: inst.id,
		Bars:       map[market.Timeframe][]market.Bar{market.Day: generateBars(inst, days)},
		// 首根 K 线之前生效的 1/1 因子，保证 QFQ 视图可用。
		Factors: []market.AdjustmentFactor{{EffectiveTime: days[0].Add(-24 * time.Hour), Numerator: 1, Denominator: 1}},
	}
}

// tradingDays 返回 anchor 当日（含）往前 count 个非周末交易日，升序、UTC 午夜。
func tradingDays(anchor time.Time, count int) []time.Time {
	day := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	days := make([]time.Time, 0, count)
	for len(days) < count {
		if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
			days = append(days, day)
		}
		day = day.AddDate(0, 0, -1)
	}
	for i, j := 0, len(days)-1; i < j; i, j = i+1, j-1 {
		days[i], days[j] = days[j], days[i]
	}
	return days
}

// latestTradingDay 返回 now 当日或之前最近的非周末交易日（UTC 午夜）。
func latestTradingDay(now time.Time) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		day = day.AddDate(0, 0, -1)
	}
	return day
}

// generateBars 用 mulberry32 种子随机游走生成日线；同一 seed 输出确定。
// 开盘 09:30 CST、收盘 15:00 CST，均换算为 UTC 微秒精度。
func generateBars(inst mockInstrument, days []time.Time) []market.Bar {
	state := inst.seed
	floor := inst.startPrice / 4
	if floor < market.Price(market.ValueScale) {
		floor = market.Price(market.ValueScale)
	}
	prevClose := inst.startPrice
	bars := make([]market.Bar, 0, len(days))
	for _, day := range days {
		close := prevClose + market.Price(float64(prevClose)*(nextFloat(&state)-0.5)*0.06)
		if close < floor {
			close = floor
		}
		open := prevClose + market.Price(float64(prevClose)*(nextFloat(&state)-0.5)*0.024)
		if open < floor {
			open = floor
		}
		high, low := open, open
		if close > high {
			high = close
		}
		if close < low {
			low = close
		}
		high += market.Price(float64(high) * nextFloat(&state) * 0.012)
		low -= market.Price(float64(low) * nextFloat(&state) * 0.012)
		if low < floor {
			low = floor
		}
		if high <= low {
			high = low + market.Price(market.ValueScale/100)
		}
		volume := volumeBase/4 + int64(nextFloat(&state)*float64(volumeBase/2))
		amount := market.Money(volume) * market.Money(close) / market.Money(market.ValueScale)
		bars = append(bars, market.Bar{
			Instrument: inst.id,
			Timeframe:  market.Day,
			OpenTime:   day.Add(90 * time.Minute),
			CloseTime:  day.Add(7 * time.Hour),
			Open:       open,
			High:       high,
			Low:        low,
			Close:      close,
			Volume:     volume,
			Amount:     amount,
			Trading:    market.Tradable,
		})
		prevClose = close
	}
	return bars
}

// nextFloat 是 mulberry32 伪随机数，返回 [0,1)。uint32 左移 61 恒为 0，
// 与 C 原版的截断行为一致，故省略该项。
func nextFloat(state *uint32) float64 {
	*state += 0x6D2B79F5
	z := *state
	z = (z ^ (z >> 15)) * (z | 1)
	z ^= z + (z ^ (z >> 7))
	return float64(z^(z>>14)) / 4294967296.0
}

func scaledYuan(yuan float64) market.Price {
	return market.Price(yuan * float64(market.ValueScale))
}
