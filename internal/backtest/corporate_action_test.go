package backtest_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/backtest"
	"trading/internal/market"
)

func TestCashDividendAndShareBonusApplyAtomicallyBeforeOpen(t *testing.T) {
	account := fundedPosition(t, 1_000)
	beforeCash := account.Cash()

	err := account.ApplyCorporateActions([]market.CorporateAction{
		{ID: "cash", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1_000},
		{ID: "bonus", Instrument: testInstrument, Kind: market.ShareDistribution, ShareNumerator: 12, ShareDenominator: 10},
	})

	require.NoError(t, err)
	position, ok := account.Position(testInstrument)
	require.True(t, ok)
	assert.EqualValues(t, 1_200, position.Quantity)
	assert.Equal(t, beforeCash+1_000_000, account.Cash())
	assert.Equal(t, market.Money(100_000_000), position.CostBasis)
}

func TestCorporateActionsRejectDuplicateAndRightsWithoutPartialMutation(t *testing.T) {
	account := fundedPosition(t, 1_000)
	beforeCash := account.Cash()
	before, _ := account.Position(testInstrument)

	err := account.ApplyCorporateActions([]market.CorporateAction{
		{ID: "cash", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1_000},
		{ID: "rights", Instrument: testInstrument, Kind: market.RightsIssue},
	})

	assert.ErrorIs(t, err, backtest.ErrUnsupportedCorporateAction)
	assert.Equal(t, beforeCash, account.Cash())
	after, _ := account.Position(testInstrument)
	assert.Equal(t, before, after)

	require.NoError(t, account.ApplyCorporateActions([]market.CorporateAction{{ID: "once", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1}}))
	require.NoError(t, account.ApplyCorporateActions([]market.CorporateAction{{ID: "twice", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1}}))
	cashAfterOnce := account.Cash()
	err = account.ApplyCorporateActions([]market.CorporateAction{{ID: "once", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1}})
	assert.ErrorIs(t, err, backtest.ErrDuplicateCorporateAction)
	assert.Equal(t, cashAfterOnce, account.Cash())
}

func TestCorporateActionsRejectMalformedBonusBeforeAnyStateChange(t *testing.T) {
	account := fundedPosition(t, 1_000)
	beforeCash := account.Cash()
	before, _ := account.Position(testInstrument)

	err := account.ApplyCorporateActions([]market.CorporateAction{
		{ID: "cash", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: 1_000},
		{ID: "bad", Instrument: testInstrument, Kind: market.ShareDistribution, ShareNumerator: 1, ShareDenominator: 0},
	})

	assert.ErrorIs(t, err, backtest.ErrInvalidCorporateAction)
	assert.Equal(t, beforeCash, account.Cash())
	after, _ := account.Position(testInstrument)
	assert.Equal(t, before, after)
}

func TestCorporateActionsFloorFractionalSharesAndKeepUnheldActionIdempotent(t *testing.T) {
	account := fundedPosition(t, 5)
	other := market.InstrumentID{Exchange: market.SSE, Code: "600001"}

	err := account.ApplyCorporateActions([]market.CorporateAction{
		{ID: "fraction", Instrument: testInstrument, Kind: market.ShareDistribution, ShareNumerator: 3, ShareDenominator: 2},
		{ID: "unheld", Instrument: other, Kind: market.CashDividend, CashPerShare: 999},
	})

	require.NoError(t, err)
	position, ok := account.Position(testInstrument)
	require.True(t, ok)
	assert.EqualValues(t, 7, position.Quantity)
	err = account.ApplyCorporateActions([]market.CorporateAction{{ID: "unheld", Instrument: other, Kind: market.CashDividend, CashPerShare: 999}})
	assert.ErrorIs(t, err, backtest.ErrDuplicateCorporateAction)
}

func TestCorporateActionsRejectInvalidIdentifiersAndKinds(t *testing.T) {
	account := fundedPosition(t, 1)
	invalidInstrument := market.InstrumentID{Exchange: market.SSE, Code: "bad"}

	for _, action := range []market.CorporateAction{
		{Instrument: testInstrument, Kind: market.CashDividend},
		{ID: "negative-cash", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: -1},
		{ID: "unknown", Instrument: testInstrument, Kind: market.CorporateActionKind(99)},
		{ID: "invalid-instrument", Instrument: invalidInstrument, Kind: market.CashDividend},
	} {
		err := account.ApplyCorporateActions([]market.CorporateAction{action})
		assert.ErrorIs(t, err, backtest.ErrInvalidCorporateAction)
	}
}

func TestCorporateActionsRejectArithmeticOverflowAtomically(t *testing.T) {
	account := newAccount(t, math.MaxInt64)
	require.NoError(t, account.ApplyFill(backtest.Fill{ID: "max", OrderID: "max", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: 1, Quantity: math.MaxInt64, Gross: math.MaxInt64}))
	before, _ := account.Position(testInstrument)

	err := account.ApplyCorporateActions([]market.CorporateAction{{ID: "overflow", Instrument: testInstrument, Kind: market.ShareDistribution, ShareNumerator: 2, ShareDenominator: 1}})

	assert.ErrorIs(t, err, backtest.ErrInvalidCorporateAction)
	after, _ := account.Position(testInstrument)
	assert.Equal(t, before, after)
}

func TestCashDividendOverflowIsRejectedBeforePublishingBatchState(t *testing.T) {
	account := newAccount(t, math.MaxInt64)
	require.NoError(t, account.ApplyFill(backtest.Fill{ID: "one-share", OrderID: "one-share", Side: backtest.Buy, Instrument: testInstrument, Time: openTime("2026-01-05"), Price: 1, Quantity: 1, Gross: 1}))
	beforeCash := account.Cash()

	err := account.ApplyCorporateActions([]market.CorporateAction{{ID: "too-large", Instrument: testInstrument, Kind: market.CashDividend, CashPerShare: math.MaxInt64}})

	assert.ErrorIs(t, err, backtest.ErrInvalidCorporateAction)
	assert.Equal(t, beforeCash, account.Cash())
}
