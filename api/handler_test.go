package api

import (
	"context"

	"github.com/gin-gonic/gin"

	"trading/business"
	"trading/model"
)

// mockStockDataService 模拟 StockDataService
type mockStockDataService struct {
	saveErr error
}

func (m *mockStockDataService) SaveHistoricalData(ctx context.Context, code string) error {
	return m.saveErr
}

func (m *mockStockDataService) AppendStockData(ctx context.Context, code string) error {
	return m.saveErr
}

// mockFinancialReportService 模拟 FinancialReportService
type mockFinancialReportService struct {
	saveErr error
}

func (m *mockFinancialReportService) SaveFinancialReportData(ctx context.Context, code string) error {
	return m.saveErr
}

func (m *mockFinancialReportService) AppendFinancialReportData(ctx context.Context, code string) error {
	return m.saveErr
}

// mockScheduler 模拟调度器
type mockScheduler struct {
	triggerErr     error
	alreadyRunning bool
}

func (m *mockScheduler) Start(ctx context.Context, hour, minute int) {}
func (m *mockScheduler) Stop()                                       {}
func (m *mockScheduler) TriggerNow(ctx context.Context) error {
	if m.alreadyRunning {
		return business.ErrSchedulerBusy
	}
	return m.triggerErr
}

// mockFinancialScheduler 模拟财报调度器
type mockFinancialScheduler struct{}

func (m *mockFinancialScheduler) Start(ctx context.Context)            {}
func (m *mockFinancialScheduler) Stop()                                {}
func (m *mockFinancialScheduler) TriggerNow(ctx context.Context) error { return nil }

// mockSignalService 模拟信号扫描服务
type mockSignalService struct {
	signal         *business.StrategySignal
	signalErr      error
	backtestResult *business.BacktestResult
	backtestErr    error
}

func (m *mockSignalService) FindBuySignals(ctx context.Context) ([]business.StrategySignal, error) {
	if m.signal == nil {
		return nil, m.signalErr
	}
	return []business.StrategySignal{*m.signal}, m.signalErr
}

func (m *mockSignalService) FindBuySignalsByStrategy(ctx context.Context, name string) (*business.StrategySignal, error) {
	return m.signal, m.signalErr
}

func (m *mockSignalService) FindFinancialReportSignals(ctx context.Context, profitThreshold float64, quarterCount int) (*business.StrategySignal, error) {
	return m.signal, m.signalErr
}

func (m *mockSignalService) Backtest(ctx context.Context, code, strategyName, cycle string) (*business.BacktestResult, error) {
	return m.backtestResult, m.backtestErr
}

// mockQueryService 模拟查询服务
type mockQueryService struct {
	prices     []*model.StockKlineDaily
	reports    []*model.FinancialReport
	pricesErr  error
	reportsErr error
}

func (m *mockQueryService) FindStockPricesByCode(ctx context.Context, code, cycle string, limit, offset int) ([]*model.StockKlineDaily, error) {
	return m.prices, m.pricesErr
}

func (m *mockQueryService) FindFinancialReportsByCode(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return m.reports, m.reportsErr
}

func setupTestRouter(svc business.StockDataService, financialSvc business.FinancialReportService, scheduler business.Scheduler, signalSvc business.SignalService, querySvc business.QueryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewStockHandler(svc, financialSvc, scheduler, &mockFinancialScheduler{}, signalSvc, querySvc, nil)
	r.POST("/api/stocks/historical", h.SaveStockHistoricalData)
	r.GET("/api/stocks/signal", h.GetStockBuySignals)
	r.GET("/api/stocks/backtest", h.GetStockBacktest)
	r.POST("/api/stocks/append", h.AppendStockData)
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	r.GET("/api/stocks/price", h.GetStockPrice)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	r.GET("/api/stocks/financial-report/signal", h.GetFinancialReportSignal)
	return r
}
