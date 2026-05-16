package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *StockHandler) GetStockBacktest(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		respondError(c, http.StatusBadRequest, "code parameter is required")
		return
	}

	strategyName := c.Query("strategy")
	if strategyName == "" {
		respondError(c, http.StatusBadRequest, "strategy parameter is required")
		return
	}

	cycle := c.Query("cycle")

	result, err := h.signalSvc.Backtest(c.Request.Context(), code, strategyName, cycle)
	if err != nil {
		respondInternalError(c, "backtest", err)
		return
	}

	respondSuccess(c, result)
}
