package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) CancelBacktestRun(c *gin.Context) {
	if c.Request.ContentLength != 0 {
		var body struct{}
		if err := strictJSON(c, &body); err != nil {
			writeApplicationError(c, "cancel body", err)
			return
		}
	}
	run, err := h.readRun(c, port.RunBacktest, false)
	if err != nil {
		writeApplicationError(c, "cancel backtest", err)
		return
	}
	if err = h.kernel.Backtests.Cancel(c.Request.Context(), run.ID); err != nil {
		writeApplicationError(c, "cancel backtest", err)
		return
	}
	respondAccepted(c, gin.H{"run_id": run.ID, "cancel_requested": true})
}
