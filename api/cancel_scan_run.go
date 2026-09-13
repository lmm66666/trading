package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) CancelScanRun(c *gin.Context) {
	if c.Request.ContentLength != 0 {
		var body struct{}
		if err := strictJSON(c, &body); err != nil {
			writeApplicationError(c, "cancel body", err)
			return
		}
	}
	run, err := h.readRun(c, port.RunScan, false)
	if err != nil {
		writeApplicationError(c, "cancel scan", err)
		return
	}
	if err = h.kernel.Scans.Cancel(c.Request.Context(), run.ID); err != nil {
		writeApplicationError(c, "cancel scan", err)
		return
	}
	respondAccepted(c, gin.H{"run_id": run.ID, "cancel_requested": true})
}
