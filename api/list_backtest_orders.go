package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) ListBacktestOrders(c *gin.Context) {
	p, err := readPage(c)
	if err != nil {
		writeApplicationError(c, "orders page", err)
		return
	}
	run, err := h.readRun(c, port.RunBacktest, true)
	if err != nil {
		writeApplicationError(c, "get orders", err)
		return
	}
	page, err := h.kernel.Runs.Orders(c.Request.Context(), run.ID, p)
	if err != nil {
		writeApplicationError(c, "get orders", err)
		return
	}
	respondSuccess(c, mapPage(page, orderDTO))
}
