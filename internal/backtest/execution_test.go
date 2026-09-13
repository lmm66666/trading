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

func TestBuyUsesNextBarOpenAndRoundsToBoardLot(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100})
	account := newAccount(t, 100_000_000)

	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 123_400, 120_000, 126_000, 10_000), account)

	assert.Equal(t, backtest.RejectNone, reason)
	assert.EqualValues(t, 800, fill.Quantity)
	assert.Equal(t, market.Price(123_400), fill.Price)
	assert.Equal(t, market.Money(98_720_000), fill.Gross)
}

func TestOrderDoesNotFillOnSignalBar(t *testing.T) {
	model := newModel(t, basicConfig())
	account := newAccount(t, 100_000_000)

	_, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-05", 100_000, 99_000, 101_000, 1_000), account)

	assert.Equal(t, backtest.RejectNotYetActive, reason)
}

func TestBuyAccountsForSlippageAndEveryFeeWithoutNegativeCash(t *testing.T) {
	model := newModel(t, backtest.Config{
		InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100,
		CommissionBPS: 10, MinimumCommission: 1_000, TransferFeeBPS: 1, SlippageBPS: 100,
	})
	account := newAccount(t, 100_000_000)

	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 100_000, 90_000, 110_000, 1_000), account)

	require.Equal(t, backtest.RejectNone, reason)
	assert.EqualValues(t, 900, fill.Quantity)
	assert.Equal(t, market.Price(101_000), fill.Price)
	assert.Equal(t, market.Money(90_900_000), fill.Gross)
	assert.Equal(t, market.Money(90_900), fill.Commission)
	assert.Equal(t, market.Money(9_090), fill.TransferFee)
	assert.Equal(t, market.Money(90_999_990), fill.TotalDebit())
	require.NoError(t, account.ApplyFill(fill))
	assert.Equal(t, market.Money(9_000_010), account.Cash())
}

func TestSlippageClampsInsideBarRange(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100, SlippageBPS: 10_000})
	account := newAccount(t, 100_000_000)
	order := backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal")

	buy, buyReason := model.Execute(order, testBar("2026-01-06", 100_000, 95_000, 105_000, 1_000), account)

	require.Equal(t, backtest.RejectNone, buyReason)
	assert.Equal(t, market.Price(105_000), buy.Price)
}

func TestExecutionRejectsUntradableLimitsAndInsufficientCash(t *testing.T) {
	model := newModel(t, basicConfig())
	order := backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal")

	limitUp := market.Price(100_000)
	_, limitReason := model.Execute(order, testBarWithLimits("2026-01-06", 100_000, 99_000, 101_000, 1_000, &limitUp, nil), newAccount(t, 100_000_000))
	_, suspendedReason := model.Execute(order, market.Bar{OpenTime: openTime("2026-01-06"), CloseTime: closeTime("2026-01-06"), Open: 100_000, High: 100_000, Low: 100_000, Close: 100_000, Volume: 0, Trading: market.Suspended}, newAccount(t, 100_000_000))
	_, cashReason := model.Execute(order, testBar("2026-01-06", 100_000, 99_000, 101_000, 1_000), newAccount(t, 999))

	assert.Equal(t, backtest.RejectLimitUp, limitReason)
	assert.Equal(t, backtest.RejectNotTradable, suspendedReason)
	assert.Equal(t, backtest.RejectInsufficientCash, cashReason)
}

func TestSellRejectsLimitDownAndCannotExceedPosition(t *testing.T) {
	model := newModel(t, basicConfig())
	account := fundedPosition(t, 500)
	limitDown := market.Price(100_000)

	_, limitReason := model.Execute(backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit"), testBarWithLimits("2026-01-06", 100_000, 99_000, 101_000, 1_000, nil, &limitDown), account)
	_, quantityReason := model.Execute(backtest.Order{ID: "too-many", Side: backtest.Sell, CreatedAt: closeTime("2026-01-05"), Quantity: 600}, testBar("2026-01-06", 100_000, 99_000, 101_000, 1_000), account)

	assert.Equal(t, backtest.RejectLimitDown, limitReason)
	assert.Equal(t, backtest.RejectInsufficientPosition, quantityReason)
}

func TestAccountRejectsDuplicateFillWithoutMutation(t *testing.T) {
	account := newAccount(t, 100_000_000)
	model := newModel(t, basicConfig())
	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 100_000, 99_000, 101_000, 1_000), account)
	require.Equal(t, backtest.RejectNone, reason)
	require.NoError(t, account.ApplyFill(fill))
	cash := account.Cash()

	err := account.ApplyFill(fill)

	assert.ErrorIs(t, err, backtest.ErrDuplicateFill)
	assert.Equal(t, cash, account.Cash())
}

func TestExecutionRejectsASecondAttemptForAnAppliedOrder(t *testing.T) {
	model := newModel(t, basicConfig())
	account := newAccount(t, 100_000_000)
	order := backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal")
	bar := testBar("2026-01-06", 100_000, 99_000, 101_000, 1_000)
	fill, reason := model.Execute(order, bar, account)
	require.Equal(t, backtest.RejectNone, reason)
	require.NoError(t, account.ApplyFill(fill))

	_, reason = model.Execute(order, bar, account)

	assert.Equal(t, backtest.RejectDuplicateFill, reason)
}

func TestSellUsesDownwardSlippageAndChargesSellOnlyDuty(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100, CommissionBPS: 10, StampDutyBPS: 10, TransferFeeBPS: 1, SlippageBPS: 100})
	account := fundedPosition(t, 500)
	order := backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit")

	fill, reason := model.Execute(order, testBar("2026-01-06", 100_000, 90_000, 110_000, 1_000), account)

	require.Equal(t, backtest.RejectNone, reason)
	assert.Equal(t, market.Price(99_000), fill.Price)
	assert.Equal(t, market.Money(49_500_000), fill.Gross)
	assert.Equal(t, market.Money(49_500), fill.Commission)
	assert.Equal(t, market.Money(49_500), fill.StampDuty)
	assert.Equal(t, market.Money(4_950), fill.TransferFee)
	require.NoError(t, account.ApplyFill(fill))
	assert.Equal(t, market.Money(99_396_050), account.Cash())
	_, open := account.Position(testInstrument)
	assert.False(t, open)
}

func TestCashFractionLimitsBuyBudgetAndCeilingFeeIsDeterministic(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 5_000, LotSize: 100, CommissionBPS: 1})
	account := newAccount(t, 100_000_000)

	fill, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 100_000, 99_000, 101_000, 1_000), account)

	require.Equal(t, backtest.RejectNone, reason)
	assert.EqualValues(t, 400, fill.Quantity) // 500 shares would need fees beyond the 50% cash budget.
	assert.Equal(t, market.Money(4_000), fill.Commission)
}

func TestExecutionRejectsMalformedOrdersBarsAndArithmeticOverflow(t *testing.T) {
	model := newModel(t, basicConfig())
	account := newAccount(t, 100_000_000)
	validBar := testBar("2026-01-06", 100_000, 99_000, 101_000, 1_000)

	_, invalidOrder := model.Execute(backtest.Order{ID: "bad", Side: backtest.Buy, CreatedAt: closeTime("2026-01-05"), Quantity: -1}, validBar, account)
	_, invalidBar := model.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), market.Bar{Instrument: testInstrument, OpenTime: openTime("2026-01-06"), CloseTime: closeTime("2026-01-06"), Open: 100, Low: 101, High: 100, Volume: 1, Trading: market.Tradable}, account)
	overflowModel := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 100, SlippageBPS: 1})
	overflowBar := testBar("2026-01-06", market.Price(math.MaxInt64), 1, market.Price(math.MaxInt64), 1)
	_, overflow := overflowModel.Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), overflowBar, account)

	assert.Equal(t, backtest.RejectInvalidOrder, invalidOrder)
	assert.Equal(t, backtest.RejectInvalidBar, invalidBar)
	assert.Equal(t, backtest.RejectArithmeticOverflow, overflow)
}

func TestExecutionModelRejectsInvalidConfigurationAtConstructionAndUse(t *testing.T) {
	_, err := backtest.NewExecutionModel(backtest.Config{})
	assert.ErrorIs(t, err, backtest.ErrInvalidConfig)

	_, reason := (backtest.ExecutionModel{}).Execute(backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal"), testBar("2026-01-06", 100_000, 99_000, 101_000, 1), newAccount(t, 100_000_000))
	assert.Equal(t, backtest.RejectInvalidConfig, reason)

}

func TestExecutionRejectsCostOverflowForAFullSell(t *testing.T) {
	model := newModel(t, backtest.Config{InitialCash: math.MaxInt64, CashFractionBPS: 10_000, LotSize: 1, MinimumCommission: math.MaxInt64})
	account := fundedPosition(t, 1)

	_, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit"), testBar("2026-01-06", 1, 1, 1, 1), account)

	assert.Equal(t, backtest.RejectArithmeticOverflow, reason)
}

func TestExecutionRejectsOrderForAnotherInstrument(t *testing.T) {
	model := newModel(t, basicConfig())
	order := backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal")
	order.Instrument = market.InstrumentID{Exchange: market.SSE, Code: "600001"}

	_, reason := model.Execute(order, testBar("2026-01-06", 100_000, 99_000, 101_000, 1), newAccount(t, 100_000_000))

	assert.Equal(t, backtest.RejectInvalidOrder, reason)
}

func TestExecutionRejectsSellWhenNoPositionExists(t *testing.T) {
	model := newModel(t, basicConfig())

	_, reason := model.Execute(backtest.NewNextOpenOrder(backtest.Sell, closeTime("2026-01-05"), "exit"), testBar("2026-01-06", 100_000, 99_000, 101_000, 1), newAccount(t, 100_000_000))

	assert.Equal(t, backtest.RejectInsufficientPosition, reason)
}

func TestConfigRejectsInvalidValuesAndOverflowProneBounds(t *testing.T) {
	for _, cfg := range []backtest.Config{
		{InitialCash: -1, CashFractionBPS: 1, LotSize: 100},
		{InitialCash: 1, CashFractionBPS: 10_001, LotSize: 100},
		{InitialCash: 1, CashFractionBPS: 1, LotSize: 0},
		{InitialCash: 1, CashFractionBPS: 1, LotSize: 100, CommissionBPS: 10_001},
		{InitialCash: 1, CashFractionBPS: 1, LotSize: 100, SlippageBPS: 10_001},
		{InitialCash: 1, CashFractionBPS: 1, LotSize: 100, HoldBars: -1},
	} {
		assert.Error(t, cfg.Validate(), cfg)
	}
}

func basicConfig() backtest.Config {
	return backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100}
}

func newModel(t *testing.T, cfg backtest.Config) backtest.ExecutionModel {
	t.Helper()
	model, err := backtest.NewExecutionModel(cfg)
	require.NoError(t, err)
	return model
}

func newAccount(t *testing.T, cash market.Money) backtest.Account {
	t.Helper()
	account, err := backtest.NewAccount(cash)
	require.NoError(t, err)
	return account
}

func fundedPosition(t *testing.T, quantity int64) backtest.Account {
	t.Helper()
	account := newAccount(t, 100_000_000)
	err := account.ApplyFill(backtest.Fill{ID: "seed", OrderID: "seed-order", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: 100_000, Quantity: quantity, Gross: market.Money(quantity * 100_000)})
	require.NoError(t, err)
	return account
}

var testInstrument = market.InstrumentID{Exchange: market.SSE, Code: "600000"}

func testBar(day string, open, low, high market.Price, volume int64) market.Bar {
	return testBarWithLimits(day, open, low, high, volume, nil, nil)
}

func testBarWithLimits(day string, open, low, high market.Price, volume int64, limitUp, limitDown *market.Price) market.Bar {
	return market.Bar{Instrument: testInstrument, Timeframe: market.Day, OpenTime: openTime(day), CloseTime: closeTime(day), Open: open, High: high, Low: low, Close: open, Volume: volume, Trading: market.Tradable, LimitUp: limitUp, LimitDown: limitDown, Version: 1}
}

func openTime(day string) time.Time {
	value, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return value.Add(9*time.Hour + 30*time.Minute)
}

func closeTime(day string) time.Time {
	value, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return value.Add(15 * time.Hour)
}
