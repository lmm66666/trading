package api

import (
	"github.com/gin-gonic/gin"
	"trading/internal/market"
	"trading/internal/strategy"
)

func strategyDTO(d strategy.Definition) gin.H {
	parameters := make(map[string]gin.H, len(d.Parameters))
	for name, s := range d.Parameters {
		parameters[name] = gin.H{"default": s.Default, "min": s.Min, "max": s.Max, "integer": s.Integer}
	}
	return gin.H{"strategy": d.ID, "version": d.Version, "primary_timeframe": timeframeName(d.PrimaryTimeframe), "warmup_bars": d.WarmupBars, "default_hold_bars": d.DefaultHoldBars, "parameters": parameters, "features": d.Features, "auxiliary": d.Auxiliary}
}
func timeframeName(tf market.Timeframe) string {
	if tf == market.Week {
		return "weekly"
	}
	return "daily"
}
func (h *StockHandler) ListStrategies(c *gin.Context) {
	definitions := h.kernel.Registry.Definitions()
	items := make([]gin.H, 0, len(definitions))
	for _, d := range definitions {
		items = append(items, strategyDTO(d))
	}
	respondSuccess(c, items)
}
