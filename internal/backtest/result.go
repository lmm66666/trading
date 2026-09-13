package backtest

import (
	"time"

	"trading/internal/market"
	"trading/internal/strategy"
)

// OrderFinalReason records the terminal outcome of a one-bar next-open order.
// Values are stable so persistence adapters can map them without inspecting
// execution internals.
type OrderFinalReason uint8

const (
	OrderPending OrderFinalReason = iota
	OrderFilled
	UnfilledNoNextBar
	UnfilledInvalidConfig
	UnfilledInvalidOrder
	UnfilledNotYetActive
	UnfilledNotTradable
	UnfilledLimitUp
	UnfilledLimitDown
	UnfilledInvalidLot
	UnfilledInsufficientCash
	UnfilledInsufficientPosition
	UnfilledDuplicateFill
	UnfilledArithmeticOverflow
	UnfilledInvalidBar
	UnfilledFeesExceedProceeds
	UnfilledAccountOverflow
	UnfilledInvalidAccountTransition
)

// EquityPoint is the account value after one bar closes. Every monetary field
// uses market.Money's scaled integer representation.
type EquityPoint struct {
	Time          time.Time
	Equity        market.Money
	Cash          market.Money
	PositionValue market.Money
}

// Trade is one completed, long-only round trip.
type Trade struct {
	Entry         Fill
	Exit          Fill
	CashDividends market.Money
	HoldingBars   int
	NetProfit     market.Money
}

// Summary contains only finite ratios. A nil ratio means its denominator is
// not defined for this run.
type Summary struct {
	TotalReturn        *float64
	AnnualizedReturn   *float64
	MaximumDrawdown    float64
	ClosedTrades       int
	WinRate            *float64
	ProfitFactor       *float64
	AverageHoldingBars *float64
	HasOpenPosition    bool
}

// Result is immutable from the engine's perspective after Run returns.
type Result struct {
	Summary       Summary
	Orders        []Order
	Fills         []Fill
	Trades        []Trade
	Equity        []EquityPoint
	FinalPosition strategy.PositionView
}
