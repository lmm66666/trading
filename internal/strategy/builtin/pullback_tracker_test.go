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

func TestPullbackTrackerRestartsAfterExpiredWindow(t *testing.T) {
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 10, MaxPullbackBars: 3})
	tracker.Advance(trackerInput{Index: 0, Close: 100, Surge: true})
	tracker.Advance(trackerInput{Index: 1, Close: 110})
	assert.False(t, tracker.Advance(trackerInput{Index: 2, Close: 98}).Signal)
	assert.Equal(t, invalid, tracker.phase)

	// The expiry bar is processed once as a new idle input. It starts a fresh
	// window but cannot also emit a pullback signal.
	assert.False(t, tracker.Advance(trackerInput{Index: 3, Close: 100, Surge: true}).Signal)
	assert.Equal(t, surge, tracker.phase)
	tracker.Advance(trackerInput{Index: 4, Close: 110})
	assert.True(t, tracker.Advance(trackerInput{Index: 5, Close: 105}).Signal)
}

func TestPullbackTrackerRestartsAfterCompletedWindow(t *testing.T) {
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 10, MaxPullbackBars: 3})
	tracker.phase = complete
	assert.False(t, tracker.Advance(trackerInput{Index: 5, Close: 100, Surge: true}).Signal)
	tracker.Advance(trackerInput{Index: 6, Close: 110})
	assert.True(t, tracker.Advance(trackerInput{Index: 7, Close: 105}).Signal)
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
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 20, MaxPullbackBars: 5, GradualDays: 3, RequireNearLow: true})
	tracker.Advance(trackerInput{Index: 0, Close: 100, Gradual: true, NearLow: true})
	tracker.Advance(trackerInput{Index: 1, Close: 102, Gradual: true})
	assert.False(t, tracker.Advance(trackerInput{Index: 2, Close: 104, Gradual: true}).Signal)
	assert.Equal(t, rally, tracker.phase)
	assert.True(t, tracker.Advance(trackerInput{Index: 3, Close: 100}).Signal)
}

func TestActiveBottomWindowExtendsOnLaterSurgeOutsideLowBand(t *testing.T) {
	tracker := newPullbackTracker(trackerConfig{MaxPullbackPct: 20, MaxPullbackBars: 5, SurgeGap: 3, RequireNearLow: true})
	tracker.Advance(trackerInput{Index: 0, Close: 100, Surge: true, NearLow: true})
	tracker.Advance(trackerInput{Index: 1, Close: 110})

	// A qualified surge within the allowed gap extends the active window even
	// after price leaves the original low band; it is never a pullback signal.
	result := tracker.Advance(trackerInput{Index: 2, Close: 108, Surge: true, NearLow: false})
	assert.False(t, result.Signal)
	assert.Equal(t, rally, result.Phase)
	assert.Equal(t, 2, tracker.lastSurgeIndex)
	assert.True(t, tracker.Advance(trackerInput{Index: 3, Close: 104}).Signal)
}
