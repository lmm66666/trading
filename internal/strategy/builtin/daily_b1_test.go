package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/strategy"
)

func TestDailyB1DefinitionDeclaresAllForwardFeaturesAndWarmup(t *testing.T) {
	definition, err := newRegistry(t).Definition(dailyB1ID, strategyVersion)
	require.NoError(t, err)
	assert.Equal(t, 30, definition.WarmupBars)
	assert.Equal(t, 10, definition.DefaultHoldBars)
	assert.Contains(t, definition.Features, dailyCloseRef)
	assert.Contains(t, definition.Features, dailyVolumeRef)
	assert.Contains(t, definition.Features, dailyVolumeMA20Ref)
	assert.Contains(t, definition.Features, dailyKDJJRef)
	assert.Contains(t, definition.Features, dailyMA20Ref)
	assert.Equal(t, strategy.ParameterSpec{Default: 2, Min: 0.01, Max: 100}, definition.Parameters[dailyVolumeRatioParam])
	assert.Equal(t, strategy.ParameterSpec{Default: 10, Min: 1, Max: 10, Integer: true}, definition.Parameters[dailyMATrendLookbackParam])
}
