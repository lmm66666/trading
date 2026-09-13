package backtest

import (
	"math"

	"trading/internal/market"
	"trading/internal/strategy"
)

// MaximumDrawdown returns the largest peak-to-trough percentage decline. A
// nonpositive peak has no meaningful percentage drawdown and contributes 0.
func MaximumDrawdown(equity []market.Money) float64 {
	var peak market.Money
	var drawdown float64
	for _, value := range equity {
		if value > peak {
			peak = value
		}
		if peak <= 0 || value >= peak {
			continue
		}
		candidate := float64(peak-value) / float64(peak)
		if candidate > drawdown {
			drawdown = candidate
		}
	}
	return drawdown
}

// CalculateMetrics uses the first equity point as the initial equity. Engine
// callers use calculateMetricsWithInitial so first-bar trading costs are not
// hidden from total return.
func CalculateMetrics(equity []EquityPoint, trades []Trade, position strategy.PositionView) Summary {
	if len(equity) == 0 {
		return calculateMetricsWithInitial(equity, trades, position, 0)
	}
	return calculateMetricsWithInitial(equity, trades, position, equity[0].Equity)
}

func calculateMetricsWithInitial(equity []EquityPoint, trades []Trade, position strategy.PositionView, initial market.Money) Summary {
	summary := Summary{ClosedTrades: len(trades), HasOpenPosition: position.Open}
	values := make([]market.Money, 0, len(equity)+1)
	if initial > 0 {
		values = append(values, initial)
	}
	for _, point := range equity {
		values = append(values, point.Equity)
	}
	summary.MaximumDrawdown = MaximumDrawdown(values)
	if initial > 0 && len(equity) > 0 {
		totalReturn := float64(equity[len(equity)-1].Equity-initial) / float64(initial)
		if finite(totalReturn) {
			summary.TotalReturn = float64Ptr(totalReturn)
			if annualized, ok := annualizedReturn(initial, equity); ok {
				summary.AnnualizedReturn = float64Ptr(annualized)
			}
		}
	}
	if len(trades) == 0 {
		return summary
	}
	var winners int
	var gains, losses float64
	var holdingTotal int64
	for _, trade := range trades {
		if trade.NetProfit > 0 {
			winners++
			gains += float64(trade.NetProfit)
		} else if trade.NetProfit < 0 {
			losses += float64(-trade.NetProfit)
		}
		holdingTotal += int64(trade.HoldingBars)
	}
	winRate := float64(winners) / float64(len(trades))
	summary.WinRate = float64Ptr(winRate)
	if losses > 0 {
		profitFactor := gains / losses
		if finite(profitFactor) {
			summary.ProfitFactor = float64Ptr(profitFactor)
		}
	}
	averageHolding := float64(holdingTotal) / float64(len(trades))
	if finite(averageHolding) {
		summary.AverageHoldingBars = float64Ptr(averageHolding)
	}
	return summary
}

func annualizedReturn(initial market.Money, equity []EquityPoint) (float64, bool) {
	if initial <= 0 || len(equity) < 2 {
		return 0, false
	}
	start, end := equity[0].Time, equity[len(equity)-1].Time
	if start.IsZero() || !end.After(start) || equity[len(equity)-1].Equity < 0 {
		return 0, false
	}
	days := end.Sub(start).Hours() / 24
	if days <= 0 {
		return 0, false
	}
	value := float64(equity[len(equity)-1].Equity) / float64(initial)
	annualized := math.Pow(value, 365.0/days) - 1
	return annualized, finite(annualized)
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func float64Ptr(value float64) *float64 { return &value }
