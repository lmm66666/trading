package api

import (
	"github.com/gin-gonic/gin"

	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

// MarketRefresh 手动刷新行情数据：请求体缺省时异步触发全量补全扫描（202）；
// 提供 exchange+code 时解析唯一匹配证券并同步刷新（200）。
func (h *StockHandler) MarketRefresh(c *gin.Context) {
	var req marketRefreshRequest
	if c.Request.ContentLength != 0 {
		if err := strictJSON(c, &req); err != nil {
			writeApplicationError(c, "decode market refresh", err)
			return
		}
	}
	if (req.Exchange == "") != (req.Code == "") {
		writeApplicationError(c, "market refresh identity", application.ErrInvalidRequest)
		return
	}
	if req.Exchange == "" {
		if h.kernel.MarketTrigger == nil {
			writeApplicationError(c, "trigger market refresh", errKernelNotConfigured)
			return
		}
		if err := h.kernel.MarketTrigger.TriggerNow(h.kernel.MarketWorkers); err != nil {
			writeApplicationError(c, "trigger market refresh", err)
			return
		}
		respondAccepted(c, gin.H{"status": "ACCEPTED"})
		return
	}
	if h.kernel.MarketIngestion == nil || h.kernel.Instruments == nil {
		writeApplicationError(c, "market refresh", errKernelNotConfigured)
		return
	}
	if err := (market.InstrumentID{Exchange: market.Exchange(req.Exchange), Code: req.Code}).Validate(); err != nil {
		writeApplicationError(c, "market refresh instrument", err)
		return
	}
	ids, err := h.kernel.Instruments.ResolveCode(c.Request.Context(), req.Code)
	if err != nil {
		writeApplicationError(c, "resolve market refresh instrument", err)
		return
	}
	if len(ids) == 0 {
		writeApplicationError(c, "resolve market refresh instrument", port.ErrMarketDataNotFound)
		return
	}
	if len(ids) > 1 {
		writeApplicationError(c, "resolve market refresh instrument", errAmbiguousInstrument)
		return
	}
	result, err := h.kernel.MarketIngestion.Refresh(c.Request.Context(), ids[0])
	if err != nil {
		writeApplicationError(c, "refresh market data", err)
		return
	}
	respondSuccess(c, result)
}

// marketRefreshRequest 行情刷新请求体；exchange 与 code 必须同时提供或同时缺省。
type marketRefreshRequest struct {
	Exchange string `json:"exchange"`
	Code     string `json:"code"`
}
