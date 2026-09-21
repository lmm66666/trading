package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

// MarketRefresh 手动刷新行情数据：请求体缺省时异步触发全量补全扫描（202）；
// 提供 exchange+code 时解析唯一匹配证券并同步刷新（200）。
func (h *StockHandler) MarketRefresh(c *gin.Context) {
	var req marketRefreshRequest
	if err := decodeOptionalJSON(c, &req); err != nil {
		writeApplicationError(c, "decode market refresh", err)
		return
	}
	if (req.Exchange == "") != (req.Code == "") {
		writeApplicationError(c, "market refresh identity", application.ErrInvalidRequest)
		return
	}
	if req.Exchange != "" {
		id := market.InstrumentID{Exchange: market.Exchange(req.Exchange), Code: req.Code}
		if err := id.Validate(); err != nil {
			writeApplicationError(c, "market refresh instrument", err)
			return
		}
	}
	if h.kernel.RemoteRefresh != nil {
		status, data, err := h.kernel.RemoteRefresh.refresh(c.Request.Context(), req)
		if err != nil {
			var upstream *updaterError
			if errors.As(err, &upstream) {
				respondError(c, upstream.status, upstream.message)
			} else {
				respondError(c, 503, "UPDATER_UNAVAILABLE")
			}
			return
		}
		c.JSON(status, response{Code: 0, Message: "success", Data: data})
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
	id := market.InstrumentID{Exchange: market.Exchange(req.Exchange), Code: req.Code}
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
	if ids[0] != id {
		writeApplicationError(c, "resolve market refresh instrument", port.ErrMarketDataNotFound)
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

// decodeOptionalJSON 与 strictJSON 保持相同边界（1MiB、拒绝未知字段和尾随值），
// 但把完全为空的请求体视为合法缺省，等价于无请求体。
func decodeOptionalJSON(c *gin.Context, dst any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return application.ErrInvalidRequest
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return application.ErrInvalidRequest
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return application.ErrInvalidRequest
	}
	return nil
}
