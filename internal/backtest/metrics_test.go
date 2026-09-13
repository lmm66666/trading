package backtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/strategy"
)

func TestMetricsExcludeOpenTradeFromWinRate(t *testing.T) {
	points := []backtest.EquityPoint{{Equity: 100}, {Equity: 110}}
	got := backtest.CalculateMetrics(points, []backtest.Trade{
		{NetProfit: 10, HoldingBars: 2}, {NetProfit: -5, HoldingBars: 4},
	}, strategy.PositionView{Open: true, Quantity: 100, HoldingBars: 3})
	require.NotNil(t, got.WinRate)
	assert.Equal(t, 0.5, *got.WinRate)
	assert.Equal(t, 2, got.ClosedTrades)
	assert.True(t, got.HasOpenPosition)
	require.NotNil(t, got.ProfitFactor)
	assert.Equal(t, 2.0, *got.ProfitFactor)
	require.NotNil(t, got.AverageHoldingBars)
	assert.Equal(t, 3.0, *got.AverageHoldingBars)
}

func TestMetricsLeaveUndefinedRatiosAbsent(t *testing.T) {
	got := backtest.CalculateMetrics([]backtest.EquityPoint{{Equity: 0}}, nil, strategy.PositionView{})
	assert.Nil(t, got.TotalReturn)
	assert.Nil(t, got.AnnualizedReturn)
	assert.Nil(t, got.WinRate)
	assert.Nil(t, got.ProfitFactor)
	assert.Nil(t, got.AverageHoldingBars)
}

func TestMaximumDrawdown(t *testing.T) {
	got := backtest.MaximumDrawdown([]market.Money{100, 120, 90, 110})
	assert.InDelta(t, 0.25, got, 1e-9)
}
