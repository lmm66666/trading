package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"trading/business"
)

// AppendStockData 手动触发数据补全扫描（异步）
func (h *StockHandler) AppendStockData(c *gin.Context) {
	if err := h.scheduler.TriggerNow(c.Request.Context()); err != nil {
		if errors.Is(err, business.ErrSchedulerBusy) {
			respondError(c, http.StatusTooManyRequests, err.Error())
			return
		}
		respondInternalError(c, "trigger stock scheduler", err)
		return
	}

	respondSuccess(c, nil)
}
