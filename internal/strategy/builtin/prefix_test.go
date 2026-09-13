package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"trading/internal/strategy"
)

func TestDailyB1PastSignalsDoNotChangeWhenFutureBarsAreAppended(t *testing.T) {
	base := fixtureDailyB1(38)
	before := replayTimelineDecisions(t, "daily_b1_buy", base, nil)
	after := replayTimelineDecisions(t, "daily_b1_buy", append(base, futureRallyBars()...), nil)
	assert.Equal(t, before, after[:len(before)])
	assert.Equal(t, strategy.EnterLong, before[34].Action)
	assert.Equal(t, "daily_b1_pullback", before[34].Reason)
}

func TestBottomSurgePastSignalsDoNotChangeWhenFutureBarsAreAppended(t *testing.T) {
	base := fixtureBottomSurge(90)
	before := replayTimelineDecisions(t, "bottom_surge_pullback", base, nil)
	after := replayTimelineDecisions(t, "bottom_surge_pullback", append(base, futureRallyBars()...), nil)
	assert.Equal(t, before, after[:len(before)])
	assert.Equal(t, strategy.EnterLong, before[70].Action)
	assert.Equal(t, "bottom_surge_pullback", before[70].Reason)
}

func TestWeeklyB1UsesDailyValueAtOrBeforeWeeklyClose(t *testing.T) {
	weeks, daily := fixtureWeeklyWithFutureDailyMove()
	decision := replayLatest(t, "weekly_b1_buy", weeks, daily)
	assert.Equal(t, strategy.Hold, decision.Action)
}

func TestWeeklyB1GoldenFixtureEntersWhenAllConfirmedFeaturesAlign(t *testing.T) {
	weeks, daily := fixtureWeeklyEntry()
	decision := replayLatest(t, "weekly_b1_buy", weeks, daily)
	assert.Equal(t, strategy.EnterLong, decision.Action)
	assert.Equal(t, "weekly_b1_alignment", decision.Reason)
}
