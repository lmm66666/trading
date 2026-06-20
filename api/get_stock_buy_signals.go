package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetStockBuySignals GET /api/stocks/signal?strategy=
// 按指定策略名称扫描所有股票，返回命中策略的股票代码列表
func (h *StockHandler) GetStockBuySignals(c *gin.Context) {
	strategyName := c.Query("strategy")
	if strategyName == "" {
		respondError(c, http.StatusBadRequest, "strategy parameter is required")
		return
	}

	result, err := h.signalSvc.FindBuySignalsByStrategy(c.Request.Context(), strategyName)
	if err != nil {
		respondInternalError(c, "find buy signals", err)
		return
	}

	if result == nil {
		respondSuccess(c, gin.H{"name": strategyName, "codes": []string{}})
		return
	}
	respondSuccess(c, result)
}
