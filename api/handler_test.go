package api

import (
	"context"

	"github.com/gin-gonic/gin"

	"trading/business"
	"trading/model"
)

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

// mockFinancialScheduler 模拟财报调度器
type mockFinancialScheduler struct{}

func (m *mockFinancialScheduler) Start(ctx context.Context)            {}
func (m *mockFinancialScheduler) Stop()                                {}
func (m *mockFinancialScheduler) TriggerNow(ctx context.Context) error { return nil }

// mockSignalService 模拟信号扫描服务
type mockSignalService struct {
	signal    *business.StrategySignal
	signalErr error
}

func (m *mockSignalService) FindFinancialReportSignals(ctx context.Context, profitThreshold float64, quarterCount int) (*business.StrategySignal, error) {
	return m.signal, m.signalErr
}

// mockQueryService 模拟查询服务
type mockQueryService struct {
	reports    []*model.FinancialReport
	reportsErr error
}

func (m *mockQueryService) FindFinancialReportsByCode(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return m.reports, m.reportsErr
}

func setupTestRouter(financialSvc business.FinancialReportService, signalSvc business.SignalService, querySvc business.QueryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewStockHandler(financialSvc, &mockFinancialScheduler{}, signalSvc, querySvc, nil)
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
