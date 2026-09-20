package api

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"trading/internal/application"
)

type updateChartBoardRequest struct {
	Name   *string          `json:"name"`
	Config *json.RawMessage `json:"config"`
}

func (h *StockHandler) UpdateChartBoard(c *gin.Context) {
	if h.kernel.ChartBoards == nil {
		writeApplicationError(c, "update chart board", errKernelNotConfigured)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		writeApplicationError(c, "parse chart board id", application.ErrInvalidRequest)
		return
	}
	var body updateChartBoardRequest
	if err := strictJSON(c, &body); err != nil {
		writeApplicationError(c, "update chart board", err)
		return
	}
	var config json.RawMessage
	if body.Config != nil {
		config = *body.Config
	}
	state, err := h.kernel.ChartBoards.Update(c.Request.Context(), id, body.Name, config)
	if err != nil {
		writeApplicationError(c, "update chart board", err)
		return
	}
	respondSuccess(c, chartBoardsDTO(state))
}
