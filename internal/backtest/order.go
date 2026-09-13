package backtest

import (
	"fmt"
	"time"

	"trading/internal/market"
)

type Side uint8

const (
	Buy Side = iota + 1
	Sell
)

// Order is an intention created after a bar closes. A zero Quantity means
// "size the buy from cash" or "sell the full position", according to Side.
type Order struct {
	ID          string
	Instrument  market.InstrumentID
	Side        Side
	Quantity    int64
	CreatedAt   time.Time
	Reason      string
	AttemptedAt time.Time
	FinalReason OrderFinalReason
}

// NewNextOpenOrder creates a deterministic order that is eligible only after
// createdAt. The engine attempts it exactly once on its next bar.
func NewNextOpenOrder(side Side, createdAt time.Time, reason string) Order {
	return Order{
		ID:        fmt.Sprintf("%d:%d:%s", side, createdAt.UnixNano(), reason),
		Side:      side,
		CreatedAt: createdAt,
		Reason:    reason,
	}
}

func (o Order) validFor(bar market.Bar) bool {
	if o.ID == "" || o.CreatedAt.IsZero() || (o.Side != Buy && o.Side != Sell) || o.Quantity < 0 {
		return false
	}
	return o.Instrument == (market.InstrumentID{}) || o.Instrument == bar.Instrument
}
