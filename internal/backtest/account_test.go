package backtest_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/backtest"
	"trading/internal/market"
)

func TestAccountPositionViewsAreDefensiveCopies(t *testing.T) {
	account := fundedPosition(t, 100)

	position, ok := account.Position(testInstrument)
	require.True(t, ok)
	position.Quantity = 0
	positions := account.Positions()
	positions[0].Quantity = 0
	restored, restoredOK := account.Position(testInstrument)

	assert.True(t, restoredOK)
	assert.EqualValues(t, 100, restored.Quantity)
}

func TestApplyFillRejectsNegativeCashAndWrongInstrument(t *testing.T) {
	account := newAccount(t, 100)

	err := account.ApplyFill(backtest.Fill{ID: "bad-cash", OrderID: "bad-cash", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: 1, Quantity: 100, Gross: 100, Commission: 1})
	assert.ErrorIs(t, err, backtest.ErrInsufficientCash)
	assert.Equal(t, market.Money(100), account.Cash())

	err = account.ApplyFill(backtest.Fill{ID: "bad-side", OrderID: "bad-side", Side: backtest.Sell, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: 1, Quantity: 1, Gross: 1})
	assert.ErrorIs(t, err, backtest.ErrInsufficientPosition)
}

func TestApplyFillPartiallySellsAndRetainsProportionalCostBasis(t *testing.T) {
	account := fundedPosition(t, 1_000)

	err := account.ApplyFill(backtest.Fill{ID: "sell-half", OrderID: "sell-half", Side: backtest.Sell, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 120_000, Quantity: 400, Gross: 48_000_000})

	require.NoError(t, err)
	position, ok := account.Position(testInstrument)
	require.True(t, ok)
	assert.EqualValues(t, 600, position.Quantity)
	assert.Equal(t, market.Money(60_000_000), position.CostBasis)
	assert.Equal(t, market.Price(100_000), position.AverageCost)
}

func TestNewAccountAndFillValidationRejectInvalidInputs(t *testing.T) {
	_, err := backtest.NewAccount(-1)
	assert.ErrorIs(t, err, backtest.ErrInsufficientCash)

	account := newAccount(t, 100_000_000)
	err = account.ApplyFill(backtest.Fill{ID: "bad", OrderID: "bad", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 100, Quantity: 1, Gross: 99})
	assert.ErrorIs(t, err, backtest.ErrInvalidFill)
}

func TestAccountRejectsDuplicateOrderAndNegativeSellCredit(t *testing.T) {
	account := fundedPosition(t, 10)
	first := backtest.Fill{ID: "first", OrderID: "shared-order", Side: backtest.Sell, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 100_000, Quantity: 1, Gross: 100_000}
	require.NoError(t, account.ApplyFill(first))

	err := account.ApplyFill(backtest.Fill{ID: "other-fill", OrderID: "shared-order", Side: backtest.Sell, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 100_000, Quantity: 1, Gross: 100_000})
	assert.ErrorIs(t, err, backtest.ErrDuplicateFill)

	err = account.ApplyFill(backtest.Fill{ID: "negative-credit", OrderID: "negative-credit", Side: backtest.Sell, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 100, Quantity: 1, Gross: 100, Commission: 101})
	assert.ErrorIs(t, err, backtest.ErrInvalidFill)
}

func TestZeroAccountInitializesPrivateMapsWithoutLeakingState(t *testing.T) {
	var account backtest.Account

	err := account.ApplyCorporateActions(nil)

	assert.NoError(t, err)
	assert.Empty(t, account.Positions())
	err = account.ApplyFill(backtest.Fill{ID: "zero-account", OrderID: "zero-account", Side: backtest.Sell, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 1, Quantity: 1, Gross: 1})
	assert.ErrorIs(t, err, backtest.ErrInsufficientPosition)
}

func TestApplyFillRejectsOverflowingFeesBeforeCashMutation(t *testing.T) {
	account := newAccount(t, math.MaxInt64)

	err := account.ApplyFill(backtest.Fill{ID: "overflow-fees", OrderID: "overflow-fees", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 1, Quantity: 1, Gross: 1, Commission: math.MaxInt64, StampDuty: math.MaxInt64, TransferFee: math.MaxInt64})

	assert.ErrorIs(t, err, backtest.ErrInvalidFill)
	assert.Equal(t, market.Money(math.MaxInt64), account.Cash())
}

func TestFillAmountViewsAndAdditionalBuyMaintainExactCostBasis(t *testing.T) {
	fill := backtest.Fill{Gross: 1_000, Commission: 11, StampDuty: 7, TransferFee: 2}
	fees, feesOK := fill.TotalFees()
	debit, debitOK := fill.TotalDebit()
	credit, creditOK := fill.NetCredit()
	assert.True(t, feesOK)
	assert.Equal(t, market.Money(20), fees)
	assert.True(t, debitOK)
	assert.Equal(t, market.Money(1_020), debit)
	assert.True(t, creditOK)
	assert.Equal(t, market.Money(980), credit)

	account := fundedPosition(t, 100)
	err := account.ApplyFill(backtest.Fill{ID: "add", OrderID: "add", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-06"), Price: 120_000, Quantity: 50, Gross: 6_000_000})
	require.NoError(t, err)
	position, ok := account.Position(testInstrument)
	require.True(t, ok)
	assert.EqualValues(t, 150, position.Quantity)
	assert.Equal(t, market.Money(16_000_000), position.CostBasis)
}

func TestPositionsAreSortedAndUnknownPositionIsAbsent(t *testing.T) {
	account := fundedPosition(t, 1)
	other := market.InstrumentID{Exchange: market.SSE, Code: "600001"}
	require.NoError(t, account.ApplyFill(backtest.Fill{ID: "other", OrderID: "other", Side: backtest.Buy, Instrument: other, Time: openTime("2026-01-06"), Price: 1, Quantity: 1, Gross: 1}))

	_, found := account.Position(market.InstrumentID{Exchange: market.SSE, Code: "600002"})
	positions := account.Positions()

	assert.False(t, found)
	require.Len(t, positions, 2)
	assert.Equal(t, testInstrument, positions[0].Instrument)
	assert.Equal(t, other, positions[1].Instrument)
}
