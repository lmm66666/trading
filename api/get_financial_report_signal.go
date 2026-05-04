package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetFinancialReportSignal GET /api/stocks/financial-report/signal?profit_threshold=0.1&quarter_count=4
// 扫描所有有财报数据的股票，返回连续多个季度净利润同比增长超过阈值的股票代码列表
func (h *StockHandler) GetFinancialReportSignal(c *gin.Context) {
	profitThreshold, err := strconv.ParseFloat(c.DefaultQuery("profit_threshold", "0.1"), 64)
	if err != nil || profitThreshold < 0 {
		respondError(c, http.StatusBadRequest, "invalid profit_threshold parameter")
		return
	}

	quarterCount, err := strconv.Atoi(c.DefaultQuery("quarter_count", "4"))
	if err != nil || quarterCount <= 0 {
		respondError(c, http.StatusBadRequest, "invalid quarter_count parameter")
		return
	}

	signal, err := h.signalSvc.FindFinancialReportSignals(c.Request.Context(), profitThreshold, quarterCount)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var codes []string
	if signal != nil {
		codes = signal.Codes
	}

	respondSuccess(c, gin.H{
		"strategy":         "financial_profit_growth",
		"profit_threshold": profitThreshold,
		"quarter_count":    quarterCount,
		"codes":            codes,
	})
}
