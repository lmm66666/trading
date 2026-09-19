package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/application"
	"trading/internal/market"
)

func (h *StockHandler) RemoveWatchlistItem(c *gin.Context) {
	if h.kernel.Watchlist == nil {
		writeApplicationError(c, "remove watchlist item", errKernelNotConfigured)
		return
	}
	id, err := market.ParseInstrumentID(c.Param("instrument"))
	if err != nil {
		writeApplicationError(c, "parse watchlist instrument", application.ErrInvalidRequest)
		return
	}
	items, err := h.kernel.Watchlist.Remove(c.Request.Context(), id)
	if err != nil {
		writeApplicationError(c, "remove watchlist item", err)
		return
	}
	respondSuccess(c, gin.H{"items": watchlistItemsDTO(items)})
}

func watchlistItemsDTO(items []application.WatchlistItem) []gin.H {
	dtos := make([]gin.H, 0, len(items))
	for _, item := range items {
		dto := instrumentSummaryDTO(item.InstrumentSummary)
		dto["close"] = item.Close
		dto["change"] = item.Change
		dto["change_pct"] = item.ChangePercent
		dtos = append(dtos, dto)
	}
	return dtos
}
