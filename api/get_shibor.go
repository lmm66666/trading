package api

import (
	"github.com/gin-gonic/gin"
)

// GetShibor GET /api/macro/shibor?period=
// 获取 Shibor 利率数据，支持按期限筛选，不指定则返回所有期限
func (h *StockHandler) GetShibor(c *gin.Context) {
	period := c.Query("period")

	data, err := h.macroSvc.GetShibor(c.Request.Context(), period)
	if err != nil {
		respondInternalError(c, "get shibor", err)
		return
	}

	respondSuccess(c, gin.H{"period": period, "data": data})
}
