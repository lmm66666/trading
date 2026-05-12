package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SaveStockHistoricalData 从 broker 获取历史数据并保存
func (h *StockHandler) SaveStockHistoricalData(c *gin.Context) {
	var req saveHistoricalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		respondError(c, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.svc.SaveHistoricalData(c.Request.Context(), req.Code); err != nil {
		respondInternalError(c, "save historical data", err)
		return
	}

	respondSuccess(c, nil)
}

// saveHistoricalRequest 保存历史数据请求体
type saveHistoricalRequest struct {
	Code string `json:"code" binding:"required"`
}
