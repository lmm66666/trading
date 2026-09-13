package backtest

import (
	"math"
	"math/bits"

	"trading/internal/market"
)

// All nonnegative divisions in this file use floor unless the name says Ceil.
// Costs charge the conservative ceiling of an exact fractional fee.
func mulDivFloor(a, b, divisor int64) (int64, bool) {
	if a < 0 || b < 0 || divisor <= 0 {
		return 0, false
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi >= uint64(divisor) {
		return 0, false
	}
	quotient, _ := bits.Div64(hi, lo, uint64(divisor))
	if quotient > math.MaxInt64 {
		return 0, false
	}
	return int64(quotient), true
}

func mulDivCeil(a, b, divisor int64) (int64, bool) {
	if a < 0 || b < 0 || divisor <= 0 {
		return 0, false
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi >= uint64(divisor) {
		return 0, false
	}
	quotient, remainder := bits.Div64(hi, lo, uint64(divisor))
	if remainder != 0 {
		quotient++
	}
	if quotient > math.MaxInt64 {
		return 0, false
	}
	return int64(quotient), true
}

func addMoney(values ...market.Money) (market.Money, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-int64(value) {
			return 0, false
		}
		total += int64(value)
	}
	return market.Money(total), true
}

func fee(gross market.Money, bps int64) (market.Money, bool) {
	value, ok := mulDivCeil(int64(gross), bps, basisPoints)
	return market.Money(value), ok
}

func tradeFees(cfg Config, side Side, gross market.Money) (commission, stampDuty, transferFee market.Money, ok bool) {
	commission, ok = fee(gross, cfg.CommissionBPS)
	if !ok {
		return 0, 0, 0, false
	}
	if commission < cfg.MinimumCommission {
		commission = cfg.MinimumCommission
	}
	transferFee, ok = fee(gross, cfg.TransferFeeBPS)
	if !ok {
		return 0, 0, 0, false
	}
	if side == Sell {
		stampDuty, ok = fee(gross, cfg.StampDutyBPS)
		if !ok {
			return 0, 0, 0, false
		}
	}
	return commission, stampDuty, transferFee, true
}

func executionPrice(open, low, high market.Price, side Side, slippageBPS int64) (market.Price, bool) {
	var price int64
	var ok bool
	if side == Buy {
		price, ok = mulDivCeil(int64(open), basisPoints+slippageBPS, basisPoints)
		if ok && price > int64(high) {
			price = int64(high)
		}
	} else {
		price, ok = mulDivFloor(int64(open), basisPoints-slippageBPS, basisPoints)
		if ok && price < int64(low) {
			price = int64(low)
		}
	}
	return market.Price(price), ok
}
