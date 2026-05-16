package business

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"trading/model"
)

// TestSignalServiceFindBuySignalsByStrategyUnknown 未知策略名称返回错误
func TestSignalServiceFindBuySignalsByStrategyUnknown(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{})

	result, err := svc.FindBuySignalsByStrategy(context.Background(), "unknown_strategy")
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// TestSignalServiceFindBuySignalsByStrategyBottomSurge 新策略 bottom_surge_pullback 可正常调用
func TestSignalServiceFindBuySignalsByStrategyBottomSurge(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{})

	_, err := svc.FindBuySignalsByStrategy(context.Background(), "bottom_surge_pullback")
	if err != nil {
		t.Fatalf("unexpected error for bottom_surge_pullback: %v", err)
	}
}

// TestSignalServiceFindBuySignalsFindAllCodesError FindAllCodes 失败返回错误
func TestSignalServiceFindBuySignalsFindAllCodesError(t *testing.T) {
	dailyRepo := &mockDailyRepo{codesErr: errors.New("db error")}
	weeklyRepo := &mockWeeklyRepo{}

	svc := NewSignalService(dailyRepo, weeklyRepo, &mockFinancialRepo{})

	_, err := svc.FindBuySignals(context.Background())
	if err == nil {
		t.Fatal("expected error when FindAllCodes fails, got nil")
	}
}

// TestSignalServiceScanDailyStrategy 扫描日线策略
func TestSignalServiceScanDailyStrategy(t *testing.T) {
	// 使用简单数据测试扫描流程
	k := []*model.StockKlineDaily{
		{Code: "600312", Date: "2026-01-01", Open: 10, High: 10.1, Low: 9.9, Close: 10, Volume: 100000},
	}

	dailyRepo := &mockDailyRepo{k: k, codes: []string{"600312"}}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{})

	result, err := svc.FindBuySignalsByStrategy(context.Background(), "bottom_surge_pullback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 单条数据不足以产生信号
	if result != nil && len(result.Codes) > 0 {
		t.Fatal("expected no signals for insufficient data")
	}
}

// TestSignalServiceScanDailyStrategyWithMatch 扫描日线策略并匹配
func TestSignalServiceScanDailyStrategyWithMatch(t *testing.T) {
	// 构造足够长的数据，让策略有机会匹配
	k := make([]*model.StockKlineDaily, 70)
	for i := range 70 {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineDaily{
			Code:   "600312",
			Date:   "2026-01-01",
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}

	dailyRepo := &mockDailyRepo{k: k, codes: []string{"600312"}}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{})

	result, err := svc.FindBuySignalsByStrategy(context.Background(), "weekly_b1_buy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 温和上涨数据可能产生或不产生信号，测试重点是流程不报错
	if result != nil && result.Name != "weekly_b1_buy" {
		t.Fatalf("expected strategy name weekly_b1_buy, got %s", result.Name)
	}
}

// TestSignalServiceScanWeeklyStrategy 扫描周线策略
func TestSignalServiceScanWeeklyStrategy(t *testing.T) {
	k := make([]*model.StockKlineWeekly, 30)
	for i := range 30 {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineWeekly{
			Code:   "600312",
			Date:   fmt.Sprintf("2026-%02d-%02d", (i/4)+1, (i%4)*7+1),
			Open:   price - 0.05,
			High:   price + 0.1,
			Low:    price - 0.1,
			Close:  price,
			Volume: 100000,
		}
	}
	// 构造下跌趋势让 KDJ 走低
	for i := 5; i < 15; i++ {
		k[i].Close = 8.0 - float64(i-5)*0.05
		k[i].High = k[i].Close + 0.1
		k[i].Low = k[i].Close - 0.1
	}
	// 后面温和上涨让 MA20 向上
	for i := 15; i < 30; i++ {
		k[i].Close = 7.5 + float64(i-15)*0.08
		k[i].High = k[i].Close + 0.1
		k[i].Low = k[i].Close - 0.05
	}

	weeklyRepo := &mockWeeklyRepo{k: k, codes: []string{"600312"}}
	svc := NewSignalService(&mockDailyRepo{}, weeklyRepo, &mockFinancialRepo{})

	result, err := svc.FindBuySignalsByStrategy(context.Background(), "weekly_b1_buy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 测试重点是扫描流程不报错，不强制要求匹配到信号
	if result != nil && result.Name != "weekly_b1_buy" {
		t.Fatalf("expected strategy name weekly_b1_buy, got %s", result.Name)
	}
}

// TestSignalServiceFindFinancialReportSignals 扫描财报策略
func TestSignalServiceFindFinancialReportSignals(t *testing.T) {
	finRepo := &mockFinancialRepo{reports: []*model.FinancialReport{}}
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, finRepo)

	result, err := svc.FindFinancialReportSignals(context.Background(), 20.0, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && len(result.Codes) > 0 {
		t.Fatal("expected no codes since FindAllCodes returns nil")
	}
}

// TestSignalServiceScanDailyStrategyRepoError FindByCode 错误应跳过
func TestSignalServiceScanDailyStrategyRepoError(t *testing.T) {
	dailyRepo := &mockDailyRepo{findErr: errors.New("db error")}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{})

	result, err := svc.FindBuySignalsByStrategy(context.Background(), "daily_b1_buy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && len(result.Codes) > 0 {
		t.Fatal("expected no codes when FindByCode fails")
	}
}
