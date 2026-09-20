package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"trading/internal/application"
)

func (h *StockHandler) ActivateChartBoard(c *gin.Context) {
	if h.kernel.ChartBoards == nil {
		writeApplicationError(c, "activate chart board", errKernelNotConfigured)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeApplicationError(c, "parse chart board id", application.ErrInvalidRequest)
		return
	}
	state, err := h.kernel.ChartBoards.Activate(c.Request.Context(), id)
	if err != nil {
		writeApplicationError(c, "activate chart board", err)
		return
	}
	respondSuccess(c, chartBoardsDTO(state))
}
