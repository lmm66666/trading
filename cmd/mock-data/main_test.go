package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"trading/config"
	"trading/internal/market"
	"trading/internal/port"
)

func TestMockInstrumentsCoverThreeStocksAndOneFuture(t *testing.T) {
	if len(mockInstruments) != 4 {
		t.Fatalf("expected 4 mock instruments, got %d", len(mockInstruments))
	}
	futures := 0
	for _, inst := range mockInstruments {
		if err := inst.id.Validate(); err != nil {
			t.Fatalf("%s invalid: %v", inst.id, err)
		}
		if inst.name == "" || inst.board == "" || inst.lotSize <= 0 || inst.startPrice <= 0 {
			t.Fatalf("%s incomplete metadata", inst.id)
		}
		if inst.id.AssetClass() == market.Futures {
			futures++
		}
	}
	if futures != 1 {
		t.Fatalf("expected exactly 1 futures instrument, got %d", futures)
	}
}

func TestTradingDaysSkipsWeekendsAndSortsAscending(t *testing.T) {
	anchor := time.Date(2026, time.September, 19, 8, 0, 0, 0, time.UTC) // 周六
	days := tradingDays(anchor, 10)
	if len(days) != 10 {
		t.Fatalf("expected 10 trading days, got %d", len(days))
	}
	for i, day := range days {
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("day %d (%s) is weekend", i, day)
		}
		if day.Location() != time.UTC || day.Hour() != 0 || day.Nanosecond() != 0 {
			t.Fatalf("day %d (%s) is not UTC midnight", i, day)
		}
		if i > 0 && !days[i-1].Before(day) {
			t.Fatalf("days not strictly ascending at %d", i)
		}
	}
	if got := days[len(days)-1]; !got.Equal(time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("anchor saturday should roll back to friday, got %s", got)
	}
}

func TestLatestTradingDayRollsBackFromWeekend(t *testing.T) {
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC), time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)}, // 周六
		{time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC), time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)}, // 周日
		{time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC), time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)}, // 周五
	}
	for _, tc := range cases {
		if got := latestTradingDay(tc.now); !got.Equal(tc.want) {
			t.Fatalf("latestTradingDay(%s) = %s, want %s", tc.now, got, tc.want)
		}
	}
}

func TestBuildBatchesPassCanonicalValidation(t *testing.T) {
	anchor := time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)
	for _, inst := range mockInstruments {
		batch := buildBatch(inst, anchor)
		canonical, digest, err := port.CanonicalMarketBatch(batch)
		if err != nil {
			t.Fatalf("%s canonical validation failed: %v", inst.id, err)
		}
		bars := canonical.Bars[market.Day]
		if len(bars) != mockBarCount {
			t.Fatalf("%s expected %d bars, got %d", inst.id, mockBarCount, len(bars))
		}
		if len(canonical.Factors) != 1 {
			t.Fatalf("%s expected 1 factor, got %d", inst.id, len(canonical.Factors))
		}
		factor := canonical.Factors[0]
		if factor.EffectiveTime.After(bars[0].CloseTime) {
			t.Fatalf("%s factor effective after first bar close", inst.id)
		}
		if factor.Numerator <= 0 || factor.Denominator <= 0 {
			t.Fatalf("%s factor not positive", inst.id)
		}
		// 同一 seed 重复生成必须得到同一 digest。
		rebuilt := buildBatch(inst, anchor)
		if _, rebuiltDigest, err := port.CanonicalMarketBatch(rebuilt); err != nil || rebuiltDigest != digest {
			t.Fatalf("%s generation not deterministic", inst.id)
		}
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	cases := [][]string{{"-unknown"},{"extra"}, {}}
	for _, args := range cases[:2] {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out, func(string) (*config.Config, error) { return &config.Config{}, nil }, nil); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}

func TestRunPublishesAndReportsOutcomes(t *testing.T) {
	var out bytes.Buffer
	loadedPath := ""
	published := false
	load := func(path string) (*config.Config, error) {
		loadedPath = path
		return &config.Config{DB: config.DB{Host: "127.0.0.1", Port: 3306, User: "root", DBName: "trading"}}, nil
	}
	publish := func(ctx context.Context, cfg *config.Config) ([]publishOutcome, error) {
		published = true
		if cfg.DB.DBName != "trading" {
			t.Fatal("config not passed through")
		}
		return []publishOutcome{{Instrument: mockInstruments[0].id, Version: 3, Bars: mockBarCount}}, nil
	}
	if err := run(context.Background(), []string{"-config", "config.yaml.local"}, &out, load, publish); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if loadedPath != "config.yaml.local" || !published {
		t.Fatalf("loader/publisher not invoked as expected: %s %v", loadedPath, published)
	}
	if !strings.Contains(out.String(), "SSE:600519 已发布 400 根日线，数据版本 3") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestLoadConfigRejectsIncompleteDatabase(t *testing.T) {
	if _, err := loadConfig("does-not-exist.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
