package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetFinancialReport GET /api/stocks/financial-report?code=xxx&pagesize=20&pagenum=1
// 根据股票代码分页查询财报数据
func (h *StockHandler) GetFinancialReport(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		respondError(c, http.StatusBadRequest, "code parameter is required")
		return
	}

	pageSize, _ := strconv.Atoi(c.DefaultQuery("pagesize", "20"))
	if pageSize <= 0 {
		pageSize = 20
	}
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pagenum", "1"))
	if pageNum <= 0 {
		pageNum = 1
	}
	offset := (pageNum - 1) * pageSize

	data, err := h.querySvc.FindFinancialReportsByCode(c.Request.Context(), code, pageSize, offset)
	if err != nil {
		respondInternalError(c, "find financial reports", err)
		return
	}

	respondSuccess(c, gin.H{"code": code, "data": data})
}
