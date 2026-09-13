package builtin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

var fixtureInstrument = market.InstrumentID{Exchange: market.SSE, Code: "600000"}

func testBar(index int, timeframe market.Timeframe, open, high, low, close float64, volume int64) market.Bar {
	closeTime := time.Date(2026, 1, 1+index, 15, 0, 0, 0, time.UTC)
	if timeframe == market.Week {
		closeTime = time.Date(2026, 1, 2+index*7, 15, 0, 0, 0, time.UTC)
	}
	return market.Bar{
		Instrument: fixtureInstrument, Timeframe: timeframe, OpenTime: closeTime.Add(-8 * time.Hour), CloseTime: closeTime,
		Open: price(open), High: price(high), Low: price(low), Close: price(close), Volume: volume,
		Trading: market.Tradable, Version: 1,
	}
}

func price(value float64) market.Price { return market.Price(value * float64(market.ValueScale)) }

func fixtureDailyB1(length int) []market.Bar {
	bars := make([]market.Bar, length)
	for index := range bars {
		close := 100 + float64(index)*0.3
		bars[index] = testBar(index, market.Day, close-0.2, close+0.5, close-0.5, close, 100)
	}
	if length > 30 {
		bars[30] = testBar(30, market.Day, 108.8, 116, 108, 115, 300)
		bars[31] = testBar(31, market.Day, 115, 119, 114, 118, 120)
		bars[32] = testBar(32, market.Day, 117, 118, 109, 110, 100)
		bars[33] = testBar(33, market.Day, 109, 110, 107, 108, 100)
		bars[34] = testBar(34, market.Day, 107, 108, 105, 106, 100)
	}
	return bars
}

func fixtureBottomSurge(length int) []market.Bar {
	bars := make([]market.Bar, length)
	for index := range bars {
		close := 100 + float64(index)*0.08
		bars[index] = testBar(index, market.Day, close-0.1, close+0.4, close-0.5, close, 100)
	}
	if length > 72 {
		bars[65] = testBar(65, market.Day, 104.8, 112, 104, 111, 260)
		bars[66] = testBar(66, market.Day, 111, 115, 110, 114, 130)
		bars[67] = testBar(67, market.Day, 113, 114, 105, 106, 100)
		bars[68] = testBar(68, market.Day, 105, 106, 102, 103, 100)
		bars[69] = testBar(69, market.Day, 102, 103, 100, 101, 100)
	}
	return bars
}

func fixtureBottomSurgeWithFollowOnSurges() []market.Bar {
	bars := fixtureBottomSurge(90)
	bars[65] = testBar(65, market.Day, 104.8, 112, 104, 111, 260)
	bars[66] = testBar(66, market.Day, 111, 115, 110, 114, 120)
	// These two qualified surge bars have left the 60-bar low band. The first
	// is within the initial gap; the second is only within the gap renewed by
	// the first, so it distinguishes a true forward extension from a restart.
	bars[67] = testBar(67, market.Day, 115, 121, 114, 120, 260)
	bars[68] = testBar(68, market.Day, 120, 122, 119, 121, 120)
	bars[69] = testBar(69, market.Day, 121, 123, 120, 122, 120)
	bars[70] = testBar(70, market.Day, 123, 130, 122, 129, 260)
	bars[71] = testBar(71, market.Day, 128, 129, 109, 112, 100)
	return bars
}

func futureRallyBars() []market.Bar {
	return []market.Bar{
		testBar(90, market.Day, 110, 114, 109, 113, 140),
		testBar(91, market.Day, 113, 118, 112, 117, 160),
	}
}

func fixtureWeeklyWithFutureDailyMove() ([]market.Bar, []market.Bar) {
	weeks := make([]market.Bar, 61)
	for index := range weeks {
		close := 90.0
		if index >= 40 {
			close = 110
		}
		weeks[index] = testBar(index, market.Week, close-0.2, close+1, close-1, close, 100)
	}
	weeks[58] = testBar(58, market.Week, 110, 111, 105, 106, 100)
	weeks[59] = testBar(59, market.Week, 106, 107, 104, 105, 100)
	weeks[60] = testBar(60, market.Week, 105, 106, 103, 104, 100)

	daily := make([]market.Bar, 471)
	for index := range daily {
		close := 110.0
		if index > 420 {
			close = 2
		}
		daily[index] = testBar(index, market.Day, close-0.1, close+0.3, close-0.3, close, 100)
	}
	return weeks, daily
}

func fixtureWeeklyEntry() ([]market.Bar, []market.Bar) {
	weeks, daily := fixtureWeeklyWithFutureDailyMove()
	for index := range daily {
		daily[index] = testBar(index, market.Day, 100, 101, 99, 100, 100)
	}
	return weeks, daily
}

// replayTimelineDecisions advances one strategy instance over one complete
// timeline. Unlike rebuilding every prefix, this preserves the production
// state-transition shape while the context still prevents future reads.
func replayTimelineDecisions(t *testing.T, id string, primaryBars, auxiliaryBars []market.Bar) []strategy.Decision {
	t.Helper()
	registry := newRegistry(t)
	instance, err := registry.Resolve(id, strategyVersion, nil)
	require.NoError(t, err)
	timeline := timelineFor(t, instance.Definition(), primaryBars, auxiliaryBars)
	decisions := make([]strategy.Decision, timeline.Len())
	for index := range decisions {
		context := &replayContext{timeline: timeline, index: index}
		decision, err := instance.OnBar(context)
		require.NoError(t, err)
		require.NoError(t, context.Err())
		decisions[index] = decision
	}
	return decisions
}

type replayContext struct {
	timeline strategy.Timeline
	index    int
	err      error
}

func (c *replayContext) Bar() market.Bar { return c.timeline.Primary.Bar(c.index) }

func (c *replayContext) Index() int { return c.index }

func (c *replayContext) Float(ref indicator.Ref, ago int) (float64, bool) {
	if ago < 0 {
		c.err = strategy.ErrFutureAccess
		return 0, false
	}
	index := c.index - ago
	if index < 0 {
		return 0, false
	}
	if ref.Timeframe == c.timeline.Primary.Timeframe() {
		series, exists := c.timeline.Features[ref.Key()]
		if !exists {
			return 0, false
		}
		return series.At(index)
	}
	aligned, exists := c.timeline.Auxiliary[ref.Timeframe]
	if !exists || index >= len(aligned.PrimaryToAuxiliary) {
		return 0, false
	}
	auxiliaryIndex := aligned.PrimaryToAuxiliary[index]
	if auxiliaryIndex < 0 {
		return 0, false
	}
	series, exists := aligned.Features[ref.Key()]
	if !exists {
		return 0, false
	}
	return series.At(auxiliaryIndex)
}

func (c *replayContext) Position() strategy.PositionView { return strategy.PositionView{} }

func (c *replayContext) Err() error { return c.err }

func replayLatest(t *testing.T, id string, primaryBars, auxiliaryBars []market.Bar) strategy.Decision {
	t.Helper()
	registry := newRegistry(t)
	instance, err := registry.Resolve(id, "1", nil)
	require.NoError(t, err)
	timeline := timelineFor(t, instance.Definition(), primaryBars, auxiliaryBars)
	decision, err := strategy.ReplayLatest(instance, timeline)
	require.NoError(t, err)
	return decision
}

func timelineFor(t *testing.T, definition strategy.Definition, primaryBars, auxiliaryBars []market.Bar) strategy.Timeline {
	t.Helper()
	primary, err := market.NewDataset(fixtureInstrument, definition.PrimaryTimeframe, 1, primaryBars)
	require.NoError(t, err)
	primaryRefs, auxiliaryRefs := splitRefs(definition)
	primaryFeatures, err := indicator.Build(primary, fixtureFactors(primary), primaryRefs)
	require.NoError(t, err)
	auxiliary := map[market.Timeframe]strategy.AlignedFeatures{}
	if len(auxiliaryRefs) > 0 {
		require.NotEmpty(t, auxiliaryBars)
		auxiliaryDataset, err := market.NewDataset(fixtureInstrument, market.Day, 1, auxiliaryBars)
		require.NoError(t, err)
		auxiliaryFeatures, err := indicator.Build(auxiliaryDataset, fixtureFactors(auxiliaryDataset), auxiliaryRefs)
		require.NoError(t, err)
		mapping := make([]int, primary.Len())
		for primaryIndex := range mapping {
			mapping[primaryIndex] = lastAsOf(primary, auxiliaryDataset, primaryIndex)
		}
		auxiliary[market.Day] = strategy.AlignedFeatures{Dataset: auxiliaryDataset, Features: auxiliaryFeatures, PrimaryToAuxiliary: mapping}
	}
	timeline, err := strategy.NewTimeline(primary, primaryFeatures, auxiliary)
	require.NoError(t, err)
	return timeline
}

func fixtureFactors(dataset market.Dataset) []market.AdjustmentFactor {
	if dataset.Len() == 0 {
		return nil
	}
	return []market.AdjustmentFactor{{EffectiveTime: dataset.Bar(0).CloseTime, Numerator: 1, Denominator: 1, Version: dataset.Version()}}
}

func splitRefs(definition strategy.Definition) ([]indicator.Ref, []indicator.Ref) {
	primary, auxiliary := make([]indicator.Ref, 0), make([]indicator.Ref, 0)
	for _, ref := range definition.Features {
		if ref.Timeframe == definition.PrimaryTimeframe {
			primary = append(primary, ref)
		} else {
			auxiliary = append(auxiliary, ref)
		}
	}
	return primary, auxiliary
}

func lastAsOf(primary, auxiliary market.Dataset, primaryIndex int) int {
	latest := -1
	for index := 0; index < auxiliary.Len(); index++ {
		if auxiliary.Bar(index).CloseTime.After(primary.Bar(primaryIndex).CloseTime) {
			break
		}
		latest = index
	}
	return latest
}

func TestFixturesAreChronological(t *testing.T) {
	for _, bars := range [][]market.Bar{fixtureDailyB1(38), fixtureBottomSurge(90)} {
		for index := 1; index < len(bars); index++ {
			require.Truef(t, bars[index-1].CloseTime.Before(bars[index].CloseTime), "bar %d", index)
		}
	}
}
