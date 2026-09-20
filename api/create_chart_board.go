package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

type createChartBoardRequest struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

func (h *StockHandler) CreateChartBoard(c *gin.Context) {
	if h.kernel.ChartBoards == nil {
		writeApplicationError(c, "create chart board", errKernelNotConfigured)
		return
	}
	var body createChartBoardRequest
	if err := strictJSON(c, &body); err != nil {
		writeApplicationError(c, "create chart board", err)
		return
	}
	state, err := h.kernel.ChartBoards.Create(c.Request.Context(), body.Name, body.Config)
	if err != nil {
		writeApplicationError(c, "create chart board", err)
		return
	}
	respondSuccess(c, chartBoardsDTO(state))
}
