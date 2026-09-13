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

// EquityDailySource is the production stock-history boundary. Weekly bars are
// derived by the application layer from these daily observations.
type EquityDailySource interface {
	FetchDailyBars(ctx context.Context, id market.InstrumentID, from, to time.Time) ([]market.Bar, error)
	FetchAdjustmentFactors(ctx context.Context, id market.InstrumentID) ([]market.AdjustmentFactor, error)
}
