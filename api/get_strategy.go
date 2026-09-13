package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

func (h *StockHandler) GetStrategy(c *gin.Context) {
	id := c.Param("strategy")
	version := c.DefaultQuery("version", "1")
	for _, f := range []struct {
		v   string
		max int
	}{{id, port.MaxStrategyIDBytes}, {version, port.MaxStrategyVersionBytes}} {
		if err := port.ValidateIdentity(f.v, "strategy", f.max, false); err != nil {
			writeApplicationError(c, "strategy", err)
			return
		}
	}
	definition, err := h.kernel.Registry.Definition(id, version)
	if err != nil {
		writeApplicationError(c, "strategy", err)
		return
	}
	respondSuccess(c, strategyDTO(definition))
}
