package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

func (h *StockHandler) SearchInstruments(c *gin.Context) {
	if h.kernel.InstrumentCatalog == nil {
		writeApplicationError(c, "search instruments", errKernelNotConfigured)
		return
	}
	queryValue, ok := singleQueryValue(c, "q", true)
	if !ok {
		writeApplicationError(c, "search instruments", application.ErrInvalidRequest)
		return
	}
	exchangeValue, ok := singleQueryValue(c, "exchange", false)
	if !ok {
		writeApplicationError(c, "search instruments", application.ErrInvalidRequest)
		return
	}
	exchange := market.Exchange(strings.ToUpper(exchangeValue))
	if exchange != "" && exchange != market.SSE && exchange != market.SZSE && exchange != market.BSE {
		writeApplicationError(c, "search instruments", application.ErrInvalidRequest)
		return
	}
	limit := 0
	if value, exists := c.Request.URL.Query()["limit"]; exists {
		if len(value) != 1 {
			writeApplicationError(c, "search instruments", application.ErrInvalidRequest)
			return
		}
		parsed, err := strconv.Atoi(value[0])
		if err != nil {
			writeApplicationError(c, "search instruments", application.ErrInvalidRequest)
			return
		}
		limit = parsed
		if limit < 1 || limit > port.MaxInstrumentSearchLimit {
			writeApplicationError(c, "search instruments", application.ErrInvalidRequest)
			return
		}
	}
	items, err := h.kernel.InstrumentCatalog.Search(c.Request.Context(), application.InstrumentSearchQuery{Query: queryValue, Exchange: exchange, Limit: limit})
	if err != nil {
		writeApplicationError(c, "search instruments", err)
		return
	}
	dtos := make([]gin.H, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, instrumentSummaryDTO(item))
	}
	respondSuccess(c, gin.H{"items": dtos})
}

func singleQueryValue(c *gin.Context, name string, required bool) (string, bool) {
	values, exists := c.Request.URL.Query()[name]
	if !exists {
		return "", !required
	}
	if len(values) != 1 || (required && strings.TrimSpace(values[0]) == "") {
		return "", false
	}
	return values[0], true
}

func instrumentSummaryDTO(item port.InstrumentSummary) gin.H {
	return gin.H{"instrument": item.ID.String(), "code": item.ID.Code, "name": item.Name, "exchange": item.ID.Exchange, "board": item.Board, "lot_size": item.LotSize}
}
