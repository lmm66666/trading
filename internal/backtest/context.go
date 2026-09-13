package backtest

import (
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

// barContext is deliberately narrow: a strategy receives the current bar,
// prior feature values, and a value snapshot of the actual account position.
type barContext struct {
	timeline strategy.Timeline
	index    int
	position strategy.PositionView
	err      error
}

func newBarContext(timeline strategy.Timeline, index int, position strategy.PositionView) *barContext {
	return &barContext{timeline: timeline, index: index, position: position}
}

func (c *barContext) Bar() market.Bar { return c.timeline.Primary.Bar(c.index) }

func (c *barContext) Index() int { return c.index }

func (c *barContext) Float(ref indicator.Ref, ago int) (float64, bool) {
	if ago < 0 {
		c.err = strategy.ErrFutureAccess
		return 0, false
	}
	index := c.index - ago
	if index < 0 {
		return 0, false
	}
	if ref.Timeframe == c.timeline.Primary.Timeframe() {
		series, ok := c.timeline.Features[ref.Key()]
		if !ok {
			return 0, false
		}
		return series.At(index)
	}
	aligned, ok := c.timeline.Auxiliary[ref.Timeframe]
	if !ok || index >= len(aligned.PrimaryToAuxiliary) {
		return 0, false
	}
	auxiliaryIndex := aligned.PrimaryToAuxiliary[index]
	if auxiliaryIndex < 0 {
		return 0, false
	}
	series, ok := aligned.Features[ref.Key()]
	if !ok {
		return 0, false
	}
	return series.At(auxiliaryIndex)
}

func (c *barContext) Position() strategy.PositionView { return c.position }

func (c *barContext) Err() error { return c.err }
