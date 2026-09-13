package backtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/market"
	"trading/internal/strategy"
)

func TestDefaultExitCountsEntryBarAsHoldingBarOne(t *testing.T) {
	bars := make([]market.Bar, 12)
	for i := range bars {
		bars[i] = engineBar("2026-01-04", 100_000, 100_000, 100_000, 100_000)
		bars[i].OpenTime = bars[i].OpenTime.AddDate(0, 0, i)
		bars[i].CloseTime = bars[i].CloseTime.AddDate(0, 0, i)
	}
	result := runScriptedBars(t, bars, append([]strategy.Action{strategy.EnterLong}, make([]strategy.Action, 10)...), nil, 10)
	require.Len(t, result.Orders, 2)
	require.Len(t, result.Fills, 2)
	assert.Equal(t, engineCloseTime("2026-01-14"), result.Orders[1].CreatedAt)
	assert.Equal(t, engineOpenTime("2026-01-15"), result.Fills[1].Time)
	assert.Equal(t, 10, result.Trades[0].HoldingBars)
}

func TestExplicitExitDoesNotDuplicateDefaultExit(t *testing.T) {
	result := runScriptedBars(t, []market.Bar{
		engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000),
		engineBar("2026-01-06", 100_000, 100_000, 100_000, 100_000),
		engineBar("2026-01-07", 100_000, 100_000, 100_000, 100_000),
	}, []strategy.Action{strategy.EnterLong, strategy.ExitLong, strategy.Hold}, nil, 1)
	require.Len(t, result.Orders, 2)
	assert.Equal(t, "bar-1", result.Orders[1].Reason)
}
