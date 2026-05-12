package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetStockPrice GET /api/stocks/price?code=xxx&cycle=daily&pagesize=20&pagenum=1
// 根据股票代码和周期查询 K 线数据，支持分页
func (h *StockHandler) GetStockPrice(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		respondError(c, http.StatusBadRequest, "code parameter is required")
		return
	}

	cycle := c.DefaultQuery("cycle", "daily")
	if cycle != "daily" && cycle != "weekly" {
		respondError(c, http.StatusBadRequest, "cycle must be daily or weekly")
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

	data, err := h.querySvc.FindStockPricesByCode(c.Request.Context(), code, cycle, pageSize, offset)
	if err != nil {
		respondInternalError(c, "find stock prices", err)
		return
	}

	respondSuccess(c, gin.H{"code": code, "cycle": cycle, "data": data})
}
