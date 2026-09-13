package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/market"
)

func TestWeeklyB1DefinitionDeclaresDailyAsOfDependency(t *testing.T) {
	definition, err := newRegistry(t).Definition(weeklyB1ID, strategyVersion)
	require.NoError(t, err)
	assert.Equal(t, 60, definition.WarmupBars)
	assert.Equal(t, []market.Timeframe{market.Day}, definition.Auxiliary)
	assert.Contains(t, definition.Features, dailyMA20Ref)
	assert.Contains(t, definition.Features, weeklyKDJJRef)
	assert.Contains(t, definition.Features, weeklyMA20Ref)
	assert.Contains(t, definition.Features, weeklyMA60Ref)
}
