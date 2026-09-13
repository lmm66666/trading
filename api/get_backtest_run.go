package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) GetBacktestRun(c *gin.Context) {
	run, err := h.readRun(c, port.RunBacktest, false)
	if err != nil {
		writeApplicationError(c, "get backtest", err)
		return
	}
	data := runDetails(run)
	if run.Status == port.RunSucceeded {
		summary, err := h.kernel.Runs.BacktestResult(c.Request.Context(), run.ID)
		if err != nil {
			writeApplicationError(c, "get backtest result", err)
			return
		}
		data["summary"] = summaryDTO(summary)
	}
	respondSuccess(c, data)
}
