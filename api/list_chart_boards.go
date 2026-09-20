package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func chartBoardsDTO(state port.ChartBoardState) gin.H {
	boards := make([]gin.H, 0, len(state.Boards))
	for _, board := range state.Boards {
		boards = append(boards, gin.H{"id": board.ID, "name": board.Name, "config": json.RawMessage(board.Config)})
	}
	return gin.H{"boards": boards, "active_id": state.ActiveID}
}

func (h *StockHandler) ListChartBoards(c *gin.Context) {
	if h.kernel.ChartBoards == nil {
		writeApplicationError(c, "list chart boards", errKernelNotConfigured)
		return
	}
	state, err := h.kernel.ChartBoards.List(c.Request.Context())
	if err != nil {
		writeApplicationError(c, "list chart boards", err)
		return
	}
	respondSuccess(c, chartBoardsDTO(state))
}
