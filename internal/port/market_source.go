package port

import (
	"context"
	"time"

	"trading/internal/market"
)

// DailyMarketSource is the production daily-history boundary. Weekly bars are
// derived by the application layer from these daily observations. Sources that
// are already continuous return one identity factor.
type DailyMarketSource interface {
	FetchDailyBars(ctx context.Context, id market.InstrumentID, from, to time.Time) ([]market.Bar, error)
	FetchAdjustmentFactors(ctx context.Context, id market.InstrumentID) ([]market.AdjustmentFactor, error)
}
