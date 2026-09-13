package backtest_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/backtest"
	"trading/internal/market"
)

func TestExecutionActivatesOnlyAfterBarOpenAndRejectsInvalidInterval(t *testing.T) {
	model := newModel(t, basicConfig())
	account := newAccount(t, 100_000_000)
	bar := testBar("2026-01-06", 100_000, 99_000, 101_000, 1)

	insideBar := backtest.NewNextOpenOrder(backtest.Buy, bar.OpenTime.Add(30*time.Minute), "signal")
	_, insideReason := model.Execute(insideBar, bar, account)
	zeroOpen := bar
	zeroOpen.OpenTime = time.Time{}
	_, zeroReason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), zeroOpen, account)
	zeroClose := bar
	zeroClose.CloseTime = time.Time{}
	_, zeroCloseReason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), zeroClose, account)
	reversed := bar
	reversed.OpenTime = reversed.CloseTime.Add(time.Second)
	_, reversedReason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), reversed, account)

	assert.Equal(t, backtest.RejectNotYetActive, insideReason)
	assert.Equal(t, backtest.RejectInvalidBar, zeroReason)
	assert.Equal(t, backtest.RejectInvalidBar, zeroCloseReason)
	assert.Equal(t, backtest.RejectInvalidBar, reversedReason)
}

func TestBuySizingHandlesMaxInt64BinarySearchBound(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 1})
	account := newAccount(t, math.MaxInt64)

	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 1, 1, 1, 1), account)

	require.Equal(t, backtest.RejectNone, reason)
	assert.EqualValues(t, math.MaxInt64, fill.Quantity)
	debit, ok := fill.TotalDebit()
	require.True(t, ok)
	assert.Equal(t, market.Money(math.MaxInt64), debit)
	require.NoError(t, account.ApplyFill(fill))
	assert.Zero(t, account.Cash())

	largeLotModel := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: math.MaxInt64})
	largeLotAccount := newAccount(t, math.MaxInt64)
	largeLotFill, largeLotReason := largeLotModel.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 1, 1, 1, 1), largeLotAccount)
	require.Equal(t, backtest.RejectNone, largeLotReason)
	assert.EqualValues(t, math.MaxInt64, largeLotFill.Quantity)
}

func TestFillAmountsAreOverflowSafeAndDirectionAware(t *testing.T) {
	fill := backtest.Fill{Gross: math.MaxInt64, Commission: 1}
	fees, feesOK := fill.TotalFees()
	debit, debitOK := fill.TotalDebit()
	credit, creditOK := fill.NetCredit()

	assert.True(t, feesOK)
	assert.Equal(t, market.Money(1), fees)
	assert.False(t, debitOK)
	assert.Zero(t, debit)
	assert.True(t, creditOK)
	assert.Equal(t, market.Money(math.MaxInt64-1), credit)
}

func TestExecuteNeverReturnsUnapplyableSellFill(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 1, CommissionBPS: 1})
	account := newAccount(t, math.MaxInt64)
	require.NoError(t, account.ApplyFill(backtest.Fill{ID: "seed", OrderID: "seed", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: math.MaxInt64, Quantity: 1, Gross: math.MaxInt64}))

	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit"), testBar("2026-01-06", math.MaxInt64, 1, math.MaxInt64, 1), account)

	require.Equal(t, backtest.RejectNone, reason)
	require.NoError(t, account.ApplyFill(fill))
	credit, ok := fill.NetCredit()
	require.True(t, ok)
	assert.Equal(t, credit, account.Cash())
}

func TestExecuteRejectsSellWhenFeesExceedGross(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 1, MinimumCommission: 2})
	account := fundedPosition(t, 1)

	_, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit"), testBar("2026-01-06", 1, 1, 1, 1), account)

	assert.Equal(t, backtest.RejectFeesExceedProceeds, reason)
}

func TestBuyRejectsWhenMinimumCommissionMakesEveryLotUnaffordable(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 1, MinimumCommission: math.MaxInt64})

	_, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 1, 1, 1, 1), newAccount(t, math.MaxInt64))

	assert.Equal(t, backtest.RejectInsufficientCash, reason)
}
