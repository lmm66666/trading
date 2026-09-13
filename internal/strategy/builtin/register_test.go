package builtin

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/strategy"
)

func newRegistry(t *testing.T) *strategy.Registry {
	t.Helper()
	registry := &strategy.Registry{}
	require.NoError(t, RegisterAll(registry))
	return registry
}

func TestRegisterAllProvidesThreeStrategies(t *testing.T) {
	registry := newRegistry(t)
	for _, id := range []string{"daily_b1_buy", "weekly_b1_buy", "bottom_surge_pullback"} {
		resolved, err := registry.Resolve(id, "1", nil)
		require.NoError(t, err)
		assert.Equal(t, id, resolved.Definition().ID)
		assert.Equal(t, "1", resolved.Definition().Version)
		assert.Equal(t, 10, resolved.Definition().DefaultHoldBars)
	}
}

func TestDailyB1RejectsUnknownAndInvalidParameters(t *testing.T) {
	registry := newRegistry(t)
	_, err := registry.Resolve("daily_b1_buy", "1", map[string]float64{"mystery": 1})
	assert.ErrorIs(t, err, strategy.ErrUnknownParameter)

	_, err = registry.Resolve("daily_b1_buy", "1", map[string]float64{"pullback_bars": 1.5})
	assert.ErrorIs(t, err, strategy.ErrInvalidParameter)

	_, err = registry.Resolve("daily_b1_buy", "1", map[string]float64{"volume_ratio": 0})
	assert.ErrorIs(t, err, strategy.ErrInvalidParameter)
}

func TestRegisterAllRejectsDuplicateRegistration(t *testing.T) {
	registry := newRegistry(t)
	err := RegisterAll(registry)
	assert.True(t, errors.Is(err, strategy.ErrDuplicateStrategy))
}
