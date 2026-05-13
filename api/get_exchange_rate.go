package api

import (
	"github.com/gin-gonic/gin"
)

// GetExchangeRate GET /api/macro/exchange-rate?code=
// 获取汇率实时数据，支持按代码筛选，不指定则返回所有预设汇率
func (h *StockHandler) GetExchangeRate(c *gin.Context) {
	code := c.Query("code")

	data, err := h.macroSvc.GetExchangeRate(c.Request.Context(), code)
	if err != nil {
		respondInternalError(c, "get exchange rate", err)
		return
	}

	respondSuccess(c, gin.H{"code": code, "data": data})
}
