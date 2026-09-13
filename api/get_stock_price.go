package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

// GetStockPrice GET /api/stocks/price?code=xxx&cycle=daily&pagesize=20&pagenum=1
// 根据股票代码和周期查询 K 线数据，支持分页
func (h *StockHandler) GetStockPrice(c *gin.Context) {
	code := c.Query("code")
	cycle := c.DefaultQuery("cycle", "daily")
	tf := market.Day
	if cycle == "weekly" {
		tf = market.Week
	} else if cycle != "daily" {
		writeApplicationError(c, "price cycle", application.ErrInvalidRequest)
		return
	}
	pageSize, err := positiveQueryInt(c.DefaultQuery("pagesize", "20"))
	if err != nil || pageSize > 1000 {
		writeApplicationError(c, "price page size", application.ErrInvalidRequest)
		return
	}
	pageNum, err := positiveQueryInt(c.DefaultQuery("pagenum", "1"))
	if err != nil || pageNum > application.MaxPriceBars/pageSize {
		writeApplicationError(c, "price page number", application.ErrInvalidRequest)
		return
	}
	if h.kernel.MarketQueries == nil || h.kernel.Instruments == nil {
		writeApplicationError(c, "price query", errKernelNotConfigured)
		return
	}
	ids, err := h.kernel.Instruments.ResolveCode(c.Request.Context(), code)
	if err != nil {
		writeApplicationError(c, "resolve price instrument", err)
		return
	}
	if len(ids) == 0 {
		writeApplicationError(c, "resolve price instrument", port.ErrMarketDataNotFound)
		return
	}
	if len(ids) > 1 {
		writeApplicationError(c, "resolve price instrument", errAmbiguousInstrument)
		return
	}
	view := market.Raw
	if value := c.DefaultQuery("view", "raw"); value == "qfq" {
		view = market.ForwardAdjusted
	} else if value != "raw" {
		writeApplicationError(c, "price view", application.ErrInvalidRequest)
		return
	}
	version, err := strconv.ParseUint(c.DefaultQuery("version", "0"), 10, 64)
	if err != nil {
		writeApplicationError(c, "price version", application.ErrInvalidRequest)
		return
	}
	now := time.Now
	if h.kernel.Clock != nil {
		now = h.kernel.Clock
	}
	result, err := h.kernel.MarketQueries.Prices(c.Request.Context(), application.PriceQuery{Instrument: ids[0], Timeframe: tf, View: view, To: now().UTC(), Version: market.DataVersion(version), Limit: pageSize * pageNum})
	if err != nil {
		writeApplicationError(c, "find stock prices", err)
		return
	}
	end := len(result.Bars) - (pageNum-1)*pageSize
	if end < 0 {
		end = 0
	}
	start := end - pageSize
	if start < 0 {
		start = 0
	}
	result.Bars = result.Bars[start:end]
	respondSuccess(c, gin.H{"code": code, "cycle": cycle, "view": view, "data_version": result.DataVersion, "data": result.Bars})
}

func positiveQueryInt(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, application.ErrInvalidRequest
	}
	return n, nil
}
