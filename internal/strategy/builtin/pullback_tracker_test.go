package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPullbackTrackerSignalsOnlyAfterConfirmedPullback(t *testing.T) {
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 15, MaxPullbackBars: 3})
	assert.False(t, tracker.Advance(trackerInput{Index: 0, Close: 100, Surge: true}).Signal)
	assert.False(t, tracker.Advance(trackerInput{Index: 1, Close: 110}).Signal)
	result := tracker.Advance(trackerInput{Index: 2, Close: 105})
	assert.True(t, result.Signal)
	assert.Equal(t, pullback, result.Phase)
}

func TestPullbackTrackerExpiresWhenDepthOrDurationIsExceeded(t *testing.T) {
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 10, MaxPullbackBars: 2})
	tracker.Advance(trackerInput{Index: 0, Close: 100, Surge: true})
	tracker.Advance(trackerInput{Index: 1, Close: 110})
	assert.False(t, tracker.Advance(trackerInput{Index: 2, Close: 98}).Signal)
	assert.Equal(t, invalid, tracker.phase)

	tracker = newPullbackTracker(trackerConfig{MaxPullbackPct: 10, MaxPullbackBars: 2})
	tracker.Advance(trackerInput{Index: 0, Close: 100, Surge: true})
	tracker.Advance(trackerInput{Index: 1, Close: 110})
	tracker.Advance(trackerInput{Index: 2, Close: 109})
	tracker.Advance(trackerInput{Index: 3, Close: 108})
	assert.False(t, tracker.Advance(trackerInput{Index: 4, Close: 107}).Signal)
	assert.Equal(t, invalid, tracker.phase)
}

func TestPullbackTrackerConfirmsThreeDayGradualSurgeWithoutFutureInput(t *testing.T) {
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 20, MaxPullbackBars: 5, GradualDays: 3})
	tracker.Advance(trackerInput{Index: 0, Close: 100, Gradual: true, NearLow: true})
	tracker.Advance(trackerInput{Index: 1, Close: 102, Gradual: true})
	assert.False(t, tracker.Advance(trackerInput{Index: 2, Close: 104, Gradual: true}).Signal)
	assert.Equal(t, rally, tracker.phase)
	assert.True(t, tracker.Advance(trackerInput{Index: 3, Close: 100}).Signal)
}
