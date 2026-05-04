package api

import (
	"github.com/gin-gonic/gin"

	"trading/business"
)

// NewRouter 创建 gin 路由
func NewRouter(svc business.StockDataService, financialSvc business.FinancialReportService, scheduler business.Scheduler, financialScheduler business.FinancialScheduler, signalSvc business.SignalService, querySvc business.QueryService) *gin.Engine {
	r := gin.Default()
	h := NewStockHandler(svc, financialSvc, scheduler, financialScheduler, signalSvc, querySvc)

	r.POST("/api/stocks/historical", h.SaveStockHistoricalData)
	r.POST("/api/stocks/append", h.AppendStockData)
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	r.GET("/api/stocks/signal", h.GetStockBuySignals)
	r.GET("/api/stocks/price", h.GetStockPrice)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	r.GET("/api/stocks/financial-report/signal", h.GetFinancialReportSignal)
	return r
}
