package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) ListBacktestEquity(c *gin.Context) {
	p, err := readPage(c)
	if err != nil {
		writeApplicationError(c, "equity page", err)
		return
	}
	run, err := h.readRun(c, port.RunBacktest, true)
	if err != nil {
		writeApplicationError(c, "get equity", err)
		return
	}
	page, err := h.kernel.Runs.Equity(c.Request.Context(), run.ID, p)
	if err != nil {
		writeApplicationError(c, "get equity", err)
		return
	}
	respondSuccess(c, mapPage(page, equityDTO))
}
