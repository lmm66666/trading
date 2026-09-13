package port

import (
	"context"
	"time"

	"trading/internal/market"
)

type MarketSource interface {
	FetchBars(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error)
	FetchCorporateActions(ctx context.Context, id market.InstrumentID) ([]market.CorporateAction, error)
}
