package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) GetScanRun(c *gin.Context) {
	run, err := h.readRun(c, port.RunScan, false)
	if err != nil {
		writeApplicationError(c, "get scan", err)
		return
	}
	data := runDetails(run)
	if run.Status == port.RunSucceeded || run.Status == port.RunPartialSucceeded {
		data["snapshot_id"] = run.ID
	}
	respondSuccess(c, data)
}
