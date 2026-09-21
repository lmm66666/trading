package api

import (
	"github.com/gin-gonic/gin"
)

// NewRouter 创建 gin 路由
func NewRouter(kernel KernelServices) *gin.Engine {
	r := gin.Default()
	h := &StockHandler{kernel: kernel}
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
	r.GET("/api/v1/watchlist", h.GetWatchlist)
	r.POST("/api/v1/watchlist", h.AddWatchlistItem)
	r.DELETE("/api/v1/watchlist/:instrument", h.RemoveWatchlistItem)
	r.POST("/api/v1/chart-queries", h.QueryChart)
	r.GET("/api/v1/chart-boards", h.ListChartBoards)
	r.POST("/api/v1/chart-boards", h.CreateChartBoard)
	r.PUT("/api/v1/chart-boards/:id", h.UpdateChartBoard)
	r.POST("/api/v1/chart-boards/:id/activate", h.ActivateChartBoard)
	r.DELETE("/api/v1/chart-boards/:id", h.DeleteChartBoard)
	r.GET("/api/v1/market/bars", h.GetMarketBars)
	r.POST("/api/v1/market/refresh", h.MarketRefresh)
	registerRefreshQueries(r, "/api/v1/market/refresh", h)
	return r
}
