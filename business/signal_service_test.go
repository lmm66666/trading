package business

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"trading/model"
	"trading/pkg/filter"
	"trading/pkg/strategy"
)

// TestSignalServiceFindBuySignalsByStrategyUnknown 未知策略名称返回错误
func TestSignalServiceFindBuySignalsByStrategyUnknown(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

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
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

	_, err := svc.FindBuySignalsByStrategy(context.Background(), "bottom_surge_pullback")
	if err != nil {
		t.Fatalf("unexpected error for bottom_surge_pullback: %v", err)
	}
}

// TestSignalServiceFindBuySignalsFindAllCodesError FindAllCodes 失败返回错误
func TestSignalServiceFindBuySignalsFindAllCodesError(t *testing.T) {
	dailyRepo := &mockDailyRepo{codesErr: errors.New("db error")}
	weeklyRepo := &mockWeeklyRepo{}

	svc := NewSignalService(dailyRepo, weeklyRepo, &mockFinancialRepo{}, nil)

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
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

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
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

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
	svc := NewSignalService(&mockDailyRepo{}, weeklyRepo, &mockFinancialRepo{}, nil)

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
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, finRepo, nil)

	result, err := svc.FindFinancialReportSignals(context.Background(), 20.0, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil && len(result.Codes) > 0 {
		t.Fatal("expected no codes since FindAllCodes returns nil")
	}
}

// TestSignalServiceScanDailyStrategyRepoError 批量查询失败返回错误
func TestSignalServiceScanDailyStrategyRepoError(t *testing.T) {
	dailyRepo := &mockDailyRepo{findErr: errors.New("db error")}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

	_, err := svc.FindBuySignalsByStrategy(context.Background(), "daily_b1_buy")
	if err == nil {
		t.Fatal("expected error when batch load fails")
	}
}

// TestBacktestUnknownStrategy 未知策略返回错误
func TestBacktestUnknownStrategy(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)
	_, err := svc.Backtest(context.Background(), "600150", "unknown", "daily")
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

// TestBacktestUnsupportedCycle 不支持的周期返回错误
func TestBacktestUnsupportedCycle(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)
	_, err := svc.Backtest(context.Background(), "600150", "daily_b1_buy", "monthly")
	if err == nil {
		t.Fatal("expected error for unsupported cycle")
	}
}

// TestBacktestDaily 日线回测返回结果
func TestBacktestDaily(t *testing.T) {
	k := make([]*model.StockKlineDaily, 70)
	for i := range 70 {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineDaily{
			Code: "600312", Date: fmt.Sprintf("2026-%02d-%02d", (i/30)+1, (i%30)+1),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}
	dailyRepo := &mockDailyRepo{k: k}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

	result, err := svc.Backtest(context.Background(), "600312", "daily_b1_buy", "daily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Code != "600312" {
		t.Fatalf("expected code 600312, got %s", result.Code)
	}
	if result.Strategy != "daily_b1_buy" {
		t.Fatalf("expected strategy daily_b1_buy, got %s", result.Strategy)
	}
	if result.Cycle != "daily" {
		t.Fatalf("expected cycle daily, got %s", result.Cycle)
	}
}

// TestBacktestWeekly 周线回测返回结果
func TestBacktestWeekly(t *testing.T) {
	k := make([]*model.StockKlineWeekly, 30)
	for i := range 30 {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineWeekly{
			Code: "600312", Date: fmt.Sprintf("2026-%02d-%02d", (i/4)+1, (i%4)*7+1),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}
	weeklyRepo := &mockWeeklyRepo{k: k}
	svc := NewSignalService(&mockDailyRepo{}, weeklyRepo, &mockFinancialRepo{}, nil)

	result, err := svc.Backtest(context.Background(), "600312", "weekly_b1_buy", "weekly")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cycle != "weekly" {
		t.Fatalf("expected cycle weekly, got %s", result.Cycle)
	}
}

// TestBacktestEmptyData 无数据时返回空信号
func TestBacktestEmptyData(t *testing.T) {
	dailyRepo := &mockDailyRepo{k: []*model.StockKlineDaily{}}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

	result, err := svc.Backtest(context.Background(), "000001", "daily_b1_buy", "daily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(result.Signals))
	}
}

// TestBacktestDefaultCycle 不传 cycle 使用策略默认周期
func TestBacktestDefaultCycle(t *testing.T) {
	k := make([]*model.StockKlineDaily, 70)
	for i := range 70 {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineDaily{
			Code: "600312", Date: fmt.Sprintf("2026-%02d-%02d", (i/30)+1, (i%30)+1),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}
	dailyRepo := &mockDailyRepo{k: k}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

	result, err := svc.Backtest(context.Background(), "600312", "daily_b1_buy", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cycle != "daily" {
		t.Fatalf("expected default cycle daily, got %s", result.Cycle)
	}
}

// TestFillDailyMA20FromData 测试使用预加载日线数据填充 AuxMA20
func TestFillDailyMA20FromData(t *testing.T) {
	dailies := []*model.StockKlineDaily{
		{Date: "2026-01-01", Close: 10},
		{Date: "2026-01-02", Close: 11},
		{Date: "2026-01-03", Close: 12},
		{Date: "2026-01-04", Close: 13},
		{Date: "2026-01-05", Close: 14},
		{Date: "2026-01-06", Close: 15},
		{Date: "2026-01-07", Close: 16},
		{Date: "2026-01-08", Close: 17},
		{Date: "2026-01-09", Close: 18},
		{Date: "2026-01-10", Close: 19},
		{Date: "2026-01-11", Close: 20},
		{Date: "2026-01-12", Close: 21},
		{Date: "2026-01-13", Close: 22},
		{Date: "2026-01-14", Close: 23},
		{Date: "2026-01-15", Close: 24},
		{Date: "2026-01-16", Close: 25},
		{Date: "2026-01-17", Close: 26},
		{Date: "2026-01-18", Close: 27},
		{Date: "2026-01-19", Close: 28},
		{Date: "2026-01-20", Close: 29},
	}

	klines := []*model.StockKline{
		{Date: "2026-01-10", Close: 19},
		{Date: "2026-01-20", Close: 29},
	}

	fillDailyMA20FromData(klines, dailies)

	if klines[0].AuxMA20 <= 0 {
		t.Fatalf("expected AuxMA20 > 0 for 2026-01-10, got %f", klines[0].AuxMA20)
	}
	if klines[1].AuxMA20 <= 0 {
		t.Fatalf("expected AuxMA20 > 0 for 2026-01-20, got %f", klines[1].AuxMA20)
	}
}

// TestFillDailyMA20FromDataEmpty 测试空数据不 panic
func TestFillDailyMA20FromDataEmpty(t *testing.T) {
	fillDailyMA20FromData(nil, nil)
	fillDailyMA20FromData([]*model.StockKline{{Date: "d1"}}, nil)
	fillDailyMA20FromData(nil, []*model.StockKlineDaily{{Date: "d1", Close: 10}})
}

// TestScanCodesFromMapMatch 测试 scanCodesFromMap 匹配最后一天信号
func TestScanCodesFromMapMatch(t *testing.T) {
	klines := make([]*model.StockKline, 10)
	for i := 0; i < 10; i++ {
		price := 10.0 + float64(i)*0.1
		klines[i] = &model.StockKline{
			Date:  fmt.Sprintf("2026-01-%02d", i+1),
			Close: price,
		}
	}

	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil).(*signalService)
	st := strategy.NewStrategy("test").AddFilter(filter.NewMATrendUp(5, 1))

	klinesMap := map[string][]*model.StockKline{
		"600312": klines,
	}

	result, err := svc.scanCodesFromMap(context.Background(), st, []string{"600312"}, klinesMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Codes) != 1 || result.Codes[0] != "600312" {
		t.Fatalf("expected match 600312, got %v", result)
	}
}

// TestScanCodesFromMapNoMatch 测试最后一天不匹配时返回空
func TestScanCodesFromMapNoMatch(t *testing.T) {
	klines := make([]*model.StockKline, 5)
	for i := 0; i < 5; i++ {
		klines[i] = &model.StockKline{
			Date:  fmt.Sprintf("2026-01-%02d", i+1),
			Close: 10.0,
		}
	}

	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil).(*signalService)
	st := strategy.NewStrategy("test").AddFilter(filter.NewMATrendUp(5, 1))

	klinesMap := map[string][]*model.StockKline{
		"600312": klines,
	}

	result, err := svc.scanCodesFromMap(context.Background(), st, []string{"600312"}, klinesMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// TestScanCodesFromMapMissingCode 测试 map 中不存在的 code 被跳过
func TestScanCodesFromMapMissingCode(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil).(*signalService)
	st := strategy.NewStrategy("test")

	result, err := svc.scanCodesFromMap(context.Background(), st, []string{"000001"}, map[string][]*model.StockKline{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for missing code, got %v", result)
	}
}

// TestScanCodesFromMapContextCancel 测试 context 取消
func TestScanCodesFromMapContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil).(*signalService)
	st := strategy.NewStrategy("test")

	_, err := svc.scanCodesFromMap(ctx, st, []string{"600312"}, map[string][]*model.StockKline{
		"600312": {{Date: "2026-01-01", Close: 10}},
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestFindBuySignals 测试同时扫描日线和周线策略（无信号数据，验证不 panic）
func TestFindBuySignals(t *testing.T) {
	dailyRepo := &mockDailyRepo{codes: []string{"600312"}, k: []*model.StockKlineDaily{{Code: "600312", Date: "2026-01-01", Open: 10, High: 10.1, Low: 9.9, Close: 10, Volume: 100000}}}
	weeklyRepo := &mockWeeklyRepo{codes: []string{"600312"}, k: []*model.StockKlineWeekly{{Code: "600312", Date: "2026-01-01", Open: 10, High: 10.1, Low: 9.9, Close: 10, Volume: 100000}}}
	svc := NewSignalService(dailyRepo, weeklyRepo, &mockFinancialRepo{}, nil)

	results, err := svc.FindBuySignals(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 数据不足以产生信号，返回空结果
	if results == nil {
		results = []StrategySignal{}
	}
	_ = results
}

// TestFindScoredSignalsByStrategyEmpty 测试无信号时返回空评分结果
func TestFindScoredSignalsByStrategyEmpty(t *testing.T) {
	dailyRepo := &mockDailyRepo{codes: []string{"600312"}, k: []*model.StockKlineDaily{{Code: "600312", Date: "2026-01-01", Open: 10, High: 10.1, Low: 9.9, Close: 10, Volume: 100000}}}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)

	result, err := svc.FindScoredSignalsByStrategy(context.Background(), "bottom_surge_pullback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(result.Signals))
	}
}

// TestFindScoredSignalsByStrategyWeekly 测试周线评分接口
func TestFindScoredSignalsByStrategyWeekly(t *testing.T) {
	weeklyRepo := &mockWeeklyRepo{codes: []string{"600312"}, k: []*model.StockKlineWeekly{{Code: "600312", Date: "2026-01-01", Open: 10, High: 10.1, Low: 9.9, Close: 10, Volume: 100000}}}
	svc := NewSignalService(&mockDailyRepo{}, weeklyRepo, &mockFinancialRepo{}, nil)

	result, err := svc.FindScoredSignalsByStrategy(context.Background(), "weekly_b1_buy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFindScoredSignalsByStrategyContextCancel 测试 context 取消
func TestFindScoredSignalsByStrategyContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil)
	_, err := svc.FindScoredSignalsByStrategy(ctx, "bottom_surge_pullback")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestScoreOneDaily 测试日线评分计算
func TestScoreOneDaily(t *testing.T) {
	k := make([]*model.StockKlineDaily, 70)
	for i := 0; i < 70; i++ {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineDaily{
			Code: "600312", Date: fmt.Sprintf("2026-%02d-%02d", (i/30)+1, (i%30)+1),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}
	dailyRepo := &mockDailyRepo{k: k}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{}, nil).(*signalService)

	ss := svc.scoreOne(context.Background(), "600312", "bottom_surge_pullback", "daily")
	if ss.Code != "600312" {
		t.Fatalf("expected code 600312, got %s", ss.Code)
	}
}

// TestScoreOneWeekly 测试周线评分计算
func TestScoreOneWeekly(t *testing.T) {
	k := make([]*model.StockKlineWeekly, 30)
	for i := 0; i < 30; i++ {
		price := 10.0 + float64(i)*0.01
		k[i] = &model.StockKlineWeekly{
			Code: "600312", Date: fmt.Sprintf("2026-%02d-%02d", (i/4)+1, (i%4)*7+1),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}
	weeklyRepo := &mockWeeklyRepo{k: k}
	svc := NewSignalService(&mockDailyRepo{}, weeklyRepo, &mockFinancialRepo{}, nil).(*signalService)

	ss := svc.scoreOne(context.Background(), "600312", "weekly_b1_buy", "weekly")
	if ss.Code != "600312" {
		t.Fatalf("expected code 600312, got %s", ss.Code)
	}
}

// TestScoreOneWithStockInfo 测试评分时获取股票名称
func TestScoreOneWithStockInfo(t *testing.T) {
	svc := NewSignalService(
		&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{},
		&mockStockInfoProvider{},
	).(*signalService)

	ss := svc.scoreOne(context.Background(), "600312", "bottom_surge_pullback", "daily")
	if ss.Name != "TestStock" {
		t.Fatalf("expected name TestStock, got %s", ss.Name)
	}
}

// TestFindFinancialReportSignalsSuccess 测试财报策略扫描成功路径
func TestFindFinancialReportSignalsSuccess(t *testing.T) {
	finRepo := &mockFinancialRepo{
		reports: []*model.FinancialReport{
			{Code: "600312", ReportDate: "20250930", ReportType: 4, NetProfit: 1000},
			{Code: "600312", ReportDate: "20250630", ReportType: 2, NetProfit: 1200},
			{Code: "600312", ReportDate: "20250331", ReportType: 1, NetProfit: 1500},
			{Code: "600312", ReportDate: "20241231", ReportType: 4, NetProfit: 1800},
		},
		codes: []string{"600312"},
	}

	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, finRepo, nil)

	result, err := svc.FindFinancialReportSignals(context.Background(), 20.0, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for no match, got %v", result)
	}
}

// TestFindFinancialReportSignalsFindAllCodesError 测试 FindAllCodes 失败
func TestFindFinancialReportSignalsFindAllCodesError(t *testing.T) {
	finRepo := &mockFinancialRepo{codesErr: errors.New("db error")}
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, finRepo, nil)

	_, err := svc.FindFinancialReportSignals(context.Background(), 20.0, 4)
	if err == nil {
		t.Fatal("expected error when FindAllCodes fails")
	}
}

// TestFillDailyMA20Error 测试 fillDailyMA20 查询失败时返回空
func TestFillDailyMA20Error(t *testing.T) {
	k := make([]*model.StockKlineWeekly, 5)
	for i := 0; i < 5; i++ {
		price := 10.0 + float64(i)*0.1
		k[i] = &model.StockKlineWeekly{
			Code: "600312", Date: fmt.Sprintf("2026-%02d-%02d", (i/4)+1, (i%4)*7+1),
			Open: price - 0.05, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 100000,
		}
	}
	weeklyRepo := &mockWeeklyRepo{k: k}
	// dailyRepo 返回错误，fillDailyMA20 应该优雅处理
	dailyRepo := &mockDailyRepo{findErr: errors.New("db error")}
	svc := NewSignalService(dailyRepo, weeklyRepo, &mockFinancialRepo{}, nil)

	result, err := svc.Backtest(context.Background(), "600312", "weekly_b1_buy", "weekly")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cycle != "weekly" {
		t.Fatalf("expected cycle weekly, got %s", result.Cycle)
	}
}

// TestIsWeekday 测试工作日判断
func TestIsWeekday(t *testing.T) {
	tests := []struct {
		date     string
		expected bool
	}{
		{"2026-01-05", true},  // Monday
		{"2026-01-09", true},  // Friday
		{"2026-01-10", false}, // Saturday
		{"2026-01-11", false}, // Sunday
	}
	for _, tt := range tests {
		d, _ := time.Parse("2006-01-02", tt.date)
		if got := isWeekday(d); got != tt.expected {
			t.Errorf("isWeekday(%s) = %v, want %v", tt.date, got, tt.expected)
		}
	}
}

// mockStockInfoProvider 模拟股票信息提供者
type mockStockInfoProvider struct{}

func (m *mockStockInfoProvider) GetName(ctx context.Context, code string) string {
	return "TestStock"
}
