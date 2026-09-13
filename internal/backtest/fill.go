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

func (f Fill) TotalFees() market.Money {
	return f.Commission + f.StampDuty + f.TransferFee
}

func (f Fill) TotalDebit() market.Money {
	return f.Gross + f.TotalFees()
}

func (f Fill) NetCredit() market.Money {
	return f.Gross - f.TotalFees()
}
