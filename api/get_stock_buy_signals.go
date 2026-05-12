package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetStockBuySignals GET /api/stocks/signal?strategy=
// 按指定策略名称扫描所有股票，返回今日出现买点的股票代码列表
func (h *StockHandler) GetStockBuySignals(c *gin.Context) {
	strategyName := c.Query("strategy")
	if strategyName == "" {
		respondError(c, http.StatusBadRequest, "strategy parameter is required")
		return
	}

	signal, err := h.signalSvc.FindBuySignalsByStrategy(c.Request.Context(), strategyName)
	if err != nil {
		respondInternalError(c, "find buy signals", err)
		return
	}

	var codes []string
	if signal != nil {
		codes = signal.Codes
	}
	respondSuccess(c, gin.H{"strategy": strategyName, "codes": codes})
}
