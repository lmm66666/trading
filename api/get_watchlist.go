package api

import (
	"github.com/gin-gonic/gin"
)

func (h *StockHandler) GetWatchlist(c *gin.Context) {
	if h.kernel.Watchlist == nil {
		writeApplicationError(c, "get watchlist", errKernelNotConfigured)
		return
	}
	items, err := h.kernel.Watchlist.List(c.Request.Context())
	if err != nil {
		writeApplicationError(c, "get watchlist", err)
		return
	}
	respondSuccess(c, gin.H{"items": watchlistItemsDTO(items)})
}
