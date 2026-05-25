package api

import (
	"github.com/gin-gonic/gin"

	"trading/business"
)

// NewRouter 创建 gin 路由
func NewRouter(svc business.StockDataService, financialSvc business.FinancialReportService, scheduler business.Scheduler, financialScheduler business.FinancialScheduler, signalSvc business.SignalService, querySvc business.QueryService, macroSvc business.MacroService) *gin.Engine {
	r := gin.Default()
	h := NewStockHandler(svc, financialSvc, scheduler, financialScheduler, signalSvc, querySvc, macroSvc)

	r.POST("/api/stocks/historical", h.SaveStockHistoricalData)
	r.POST("/api/stocks/append", h.AppendStockData)
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	r.GET("/api/stocks/signal", h.GetStockBuySignals)
	r.GET("/api/stocks/signal/scored", h.GetStockScoredSignals)
	r.GET("/api/stocks/backtest", h.GetStockBacktest)
	r.GET("/api/stocks/price", h.GetStockPrice)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	r.GET("/api/stocks/financial-report/signal", h.GetFinancialReportSignal)
	r.GET("/api/macro/shibor", h.GetShibor)
	r.GET("/api/macro/exchange-rate", h.GetExchangeRate)
	return r
}
