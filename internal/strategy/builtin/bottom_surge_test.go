package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/strategy"
)

func TestBottomSurgeDefinitionCarriesCompleteDefaultContract(t *testing.T) {
	definition, err := newRegistry(t).Definition(bottomSurgeID, strategyVersion)
	require.NoError(t, err)
	assert.Equal(t, 60, definition.WarmupBars)
	assert.Equal(t, 10, definition.DefaultHoldBars)
	assert.Contains(t, definition.Features, bottomOpenRef)
	assert.Contains(t, definition.Features, bottomLowRef)
	assert.Contains(t, definition.Features, bottomMA60Ref)
	assert.Equal(t, strategy.ParameterSpec{Default: 3, Min: 0, Max: 30, Integer: true}, definition.Parameters[bottomSurgeGapParam])
	assert.Equal(t, strategy.ParameterSpec{Default: -20, Min: -200, Max: 200}, definition.Parameters[bottomJMinParam])
	assert.Equal(t, strategy.ParameterSpec{Default: 20, Min: -200, Max: 200}, definition.Parameters[bottomJMaxParam])
}

func TestBottomSurgeRejectsFractionalAndOutOfRangeParameters(t *testing.T) {
	registry := newRegistry(t)
	_, err := registry.Resolve(bottomSurgeID, strategyVersion, map[string]float64{bottomGradualDaysParam: 2.5})
	assert.ErrorIs(t, err, strategy.ErrInvalidParameter)
	_, err = registry.Resolve(bottomSurgeID, strategyVersion, map[string]float64{bottomPullbackPctParam: 101})
	assert.ErrorIs(t, err, strategy.ErrInvalidParameter)
}
