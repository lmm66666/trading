package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"trading/business"
)

// AppendFinancialReportData 手动触发财报数据补全扫描（异步）
func (h *StockHandler) AppendFinancialReportData(c *gin.Context) {
	if err := h.financialScheduler.TriggerNow(c.Request.Context()); err != nil {
		if errors.Is(err, business.ErrSchedulerBusy) {
			respondError(c, http.StatusTooManyRequests, err.Error())
			return
		}
		respondInternalError(c, "trigger financial scheduler", err)
		return
	}

	respondSuccess(c, nil)
}
