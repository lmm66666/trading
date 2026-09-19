package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/application"
	"trading/internal/market"
)

type addWatchlistRequest struct {
	Instrument string `json:"instrument"`
}

func (h *StockHandler) AddWatchlistItem(c *gin.Context) {
	if h.kernel.Watchlist == nil {
		writeApplicationError(c, "add watchlist item", errKernelNotConfigured)
		return
	}
	var body addWatchlistRequest
	if err := strictJSON(c, &body); err != nil {
		writeApplicationError(c, "decode watchlist add", err)
		return
	}
	id, err := market.ParseInstrumentID(body.Instrument)
	if err != nil {
		writeApplicationError(c, "parse watchlist instrument", application.ErrInvalidRequest)
		return
	}
	items, err := h.kernel.Watchlist.Add(c.Request.Context(), id)
	if err != nil {
		writeApplicationError(c, "add watchlist item", err)
		return
	}
	respondSuccess(c, gin.H{"items": watchlistItemsDTO(items)})
}
