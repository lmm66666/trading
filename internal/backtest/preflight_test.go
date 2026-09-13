package backtest_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/backtest"
	"trading/internal/market"
)

func TestExecuteRejectsSellThatWouldOverflowExistingCash(t *testing.T) {
	account := newAccount(t, math.MaxInt64)
	require.NoError(t, account.ApplyFill(backtest.Fill{ID: "seed", OrderID: "seed", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: math.MaxInt64, Quantity: 1, Gross: math.MaxInt64}))
	require.NoError(t, account.ApplyCorporateActions([]market.CorporateAction{{ID: "restore-cash", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: math.MaxInt64}}))
	positionBefore, ok := account.Position(testInstrument)
	require.True(t, ok)

	model := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 1})
	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit"), testBar("2026-01-06", 1, 1, 1, 1), account)

	assert.Equal(t, backtest.RejectAccountOverflow, reason)
	assert.Equal(t, backtest.Fill{}, fill)
	assert.Equal(t, market.Money(math.MaxInt64), account.Cash())
	positionAfter, ok := account.Position(testInstrument)
	assert.True(t, ok)
	assert.Equal(t, positionBefore, positionAfter)
}

func TestExecuteRejectsBuyThatWouldOverflowExistingPosition(t *testing.T) {
	account := newAccount(t, math.MaxInt64)
	require.NoError(t, account.ApplyFill(backtest.Fill{ID: "seed", OrderID: "seed", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: 1, Quantity: math.MaxInt64 - 1, Gross: math.MaxInt64 - 1}))
	require.NoError(t, account.ApplyCorporateActions([]market.CorporateAction{{ID: "restore-cash", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1}}))
	positionBefore, ok := account.Position(testInstrument)
	require.True(t, ok)

	model := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 1})
	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 1, 1, 1, 1), account)

	assert.Equal(t, backtest.RejectAccountOverflow, reason)
	assert.Equal(t, backtest.Fill{}, fill)
	assert.Equal(t, market.Money(math.MaxInt64), account.Cash())
	positionAfter, ok := account.Position(testInstrument)
	assert.True(t, ok)
	assert.Equal(t, positionBefore, positionAfter)
}
