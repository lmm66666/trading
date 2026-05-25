package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetStockScoredSignals GET /api/stocks/signal/scored?strategy=
// 按指定策略名称扫描所有股票，返回带评分的结果，按短线评分降序
func (h *StockHandler) GetStockScoredSignals(c *gin.Context) {
	strategyName := c.Query("strategy")
	if strategyName == "" {
		respondError(c, http.StatusBadRequest, "strategy parameter is required")
		return
	}

	result, err := h.signalSvc.FindScoredSignalsByStrategy(c.Request.Context(), strategyName)
	if err != nil {
		respondInternalError(c, "find scored signals", err)
		return
	}

	if result == nil {
		respondSuccess(c, gin.H{"strategy": strategyName, "signals": []any{}})
		return
	}
	respondSuccess(c, result)
}
