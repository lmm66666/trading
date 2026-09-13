package backtest

import (
	"time"
	"trading/internal/market"
)

// Fill records an executed order entirely in scaled integer units.
type Fill struct {
	ID          string
	OrderID     string
	Side        Side
	Instrument  market.InstrumentID
	Time        time.Time
	Price       market.Price
	Quantity    int64
	Gross       market.Money
	Commission  market.Money
	StampDuty   market.Money
	TransferFee market.Money
}

// TotalFees returns the exact fee total when it fits in market.Money.
func (f Fill) TotalFees() (market.Money, bool) {
	return addMoney(f.Commission, f.StampDuty, f.TransferFee)
}

// TotalDebit returns gross plus fees for a buy fill without wrapping.
func (f Fill) TotalDebit() (market.Money, bool) {
	fees, ok := f.TotalFees()
	if !ok {
		return 0, false
	}
	return addMoney(f.Gross, fees)
}

// NetCredit returns gross minus fees for a sell fill without wrapping.
func (f Fill) NetCredit() (market.Money, bool) {
	fees, ok := f.TotalFees()
	if !ok || fees > f.Gross {
		return 0, false
	}
	return f.Gross - fees, true
}
