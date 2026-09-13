package api

import "github.com/gin-gonic/gin"

// AppendStockData 手动触发数据补全扫描（异步）
func (h *StockHandler) AppendStockData(c *gin.Context) {
	if h.kernel.MarketTrigger == nil {
		writeApplicationError(c, "trigger market refresh", errKernelNotConfigured)
		return
	}
	if err := h.kernel.MarketTrigger.TriggerNow(h.kernel.MarketWorkers); err != nil {
		writeApplicationError(c, "trigger market refresh", err)
		return
	}
	respondAccepted(c, gin.H{"status": "ACCEPTED"})
}
