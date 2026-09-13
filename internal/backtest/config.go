// Package backtest provides deterministic, fixed-point execution primitives.
package backtest

import (
	"errors"
	"fmt"

	"trading/internal/market"
)

const basisPoints int64 = 10_000

var ErrInvalidConfig = errors.New("backtest: invalid configuration")

// Config controls a single-instrument, cash-only backtest. All monetary
// values are market.Money values, scaled by market.ValueScale.
type Config struct {
	InitialCash       market.Money
	CashFractionBPS   int64
	CommissionBPS     int64
	MinimumCommission market.Money
	StampDutyBPS      int64
	TransferFeeBPS    int64
	SlippageBPS       int64
	LotSize           int64
	HoldBars          int
}

// Validate rejects values which could make an execution ambiguous or unsafe.
func (c Config) Validate() error {
	if c.InitialCash < 0 || c.MinimumCommission < 0 || c.CashFractionBPS < 1 || c.CashFractionBPS > basisPoints ||
		c.CommissionBPS < 0 || c.CommissionBPS > basisPoints || c.StampDutyBPS < 0 || c.StampDutyBPS > basisPoints ||
		c.TransferFeeBPS < 0 || c.TransferFeeBPS > basisPoints || c.SlippageBPS < 0 || c.SlippageBPS > basisPoints ||
		c.LotSize <= 0 || c.HoldBars < 0 {
		return fmt.Errorf("%w", ErrInvalidConfig)
	}
	return nil
}
