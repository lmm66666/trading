package api

import (
	"github.com/gin-gonic/gin"

	"trading/business"
)

// NewRouter 创建 gin 路由
func NewRouter(financialSvc business.FinancialReportService, financialScheduler business.FinancialScheduler, signalSvc business.SignalService, querySvc business.QueryService, macroSvc business.MacroService, kernel ...KernelServices) *gin.Engine {
	r := gin.Default()
	h := NewStockHandler(financialSvc, financialScheduler, signalSvc, querySvc, macroSvc)
	if len(kernel) > 0 {
		h.kernel = kernel[0]
		r.POST("/api/v1/backtest-runs", h.CreateBacktestRun)
		r.GET("/api/v1/backtest-runs/:run_id", h.GetBacktestRun)
		r.POST("/api/v1/backtest-runs/:run_id/cancel", h.CancelBacktestRun)
		r.GET("/api/v1/backtest-runs/:run_id/orders", h.ListBacktestOrders)
		r.GET("/api/v1/backtest-runs/:run_id/trades", h.ListBacktestTrades)
		r.GET("/api/v1/backtest-runs/:run_id/equity", h.ListBacktestEquity)
		r.POST("/api/v1/scan-runs", h.CreateScanRun)
		r.GET("/api/v1/scan-runs/:run_id", h.GetScanRun)
		r.POST("/api/v1/scan-runs/:run_id/cancel", h.CancelScanRun)
		r.GET("/api/v1/signal-snapshots/latest", h.GetLatestSignalSnapshot)
		r.GET("/api/v1/strategies", h.ListStrategies)
		r.GET("/api/v1/strategies/:strategy", h.GetStrategy)
		r.GET("/api/v1/instruments", h.SearchInstruments)
	}

	r.POST("/api/stocks/historical", h.SaveStockHistoricalData)
	r.POST("/api/stocks/append", h.AppendStockData)
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	r.GET("/api/stocks/signal", h.GetStockBuySignals)
	r.GET("/api/stocks/backtest", h.GetStockBacktest)
	r.GET("/api/stocks/price", h.GetStockPrice)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	r.GET("/api/stocks/financial-report/signal", h.GetFinancialReportSignal)
	r.GET("/api/macro/shibor", h.GetShibor)
	r.GET("/api/macro/exchange-rate", h.GetExchangeRate)
	return r
}
