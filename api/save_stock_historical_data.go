package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

// SaveStockHistoricalData 从 broker 获取历史数据并保存
func (h *StockHandler) SaveStockHistoricalData(c *gin.Context) {
	var req saveHistoricalRequest
	if err := strictJSON(c, &req); err != nil {
		writeApplicationError(c, "decode historical refresh", err)
		return
	}
	if h.kernel.MarketIngestion == nil || h.kernel.Instruments == nil {
		writeApplicationError(c, "historical refresh", errKernelNotConfigured)
		return
	}
	ids, err := h.kernel.Instruments.ResolveCode(c.Request.Context(), req.Code)
	if err != nil {
		writeApplicationError(c, "resolve historical instrument", err)
		return
	}
	if len(ids) == 0 {
		writeApplicationError(c, "resolve historical instrument", port.ErrMarketDataNotFound)
		return
	}
	if len(ids) > 1 {
		writeApplicationError(c, "resolve historical instrument", errAmbiguousInstrument)
		return
	}
	result, err := h.kernel.MarketIngestion.Refresh(c.Request.Context(), ids[0])
	if err != nil {
		writeApplicationError(c, "refresh historical data", err)
		return
	}
	respondSuccess(c, result)
}

// saveHistoricalRequest 保存历史数据请求体
type saveHistoricalRequest struct {
	Code string `json:"code" binding:"required"`
}
