package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) ListBacktestTrades(c *gin.Context) {
	p, err := readPage(c)
	if err != nil {
		writeApplicationError(c, "trades page", err)
		return
	}
	run, err := h.readRun(c, port.RunBacktest, true)
	if err != nil {
		writeApplicationError(c, "get trades", err)
		return
	}
	page, err := h.kernel.Runs.Trades(c.Request.Context(), run.ID, p)
	if err != nil {
		writeApplicationError(c, "get trades", err)
		return
	}
	respondSuccess(c, mapPage(page, fillDTO))
}
