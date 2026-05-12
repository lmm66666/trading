package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SaveFinancialReportData 从 broker 获取财报数据并保存
func (h *StockHandler) SaveFinancialReportData(c *gin.Context) {
	var req saveFinancialReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		respondError(c, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.financialSvc.SaveFinancialReportData(c.Request.Context(), req.Code); err != nil {
		respondInternalError(c, "save financial report data", err)
		return
	}

	respondSuccess(c, nil)
}

// saveFinancialReportRequest 保存财报数据请求体
type saveFinancialReportRequest struct {
	Code string `json:"code" binding:"required"`
}
