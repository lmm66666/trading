package port

import (
	"context"

	"trading/internal/market"
)

// MaxWatchlistItems bounds the single-user watchlist size.
const MaxWatchlistItems = 100

// WatchlistEntry pairs a watchlisted instrument identity with its t_instruments
// row ID for quote enrichment.
type WatchlistEntry struct {
	InstrumentSummary
	InstrumentRowID uint64
}

// WatchlistStore persists the single-user watchlist in insertion order.
type WatchlistStore interface {
	List(ctx context.Context) ([]WatchlistEntry, error)
	Add(ctx context.Context, id market.InstrumentID) (exists bool, err error)
	Remove(ctx context.Context, id market.InstrumentID) error
	Count(ctx context.Context) (int, error)
}

// DailyQuote carries yuan-denominated daily close and change values; nil fields
// mean the corresponding bar data is unavailable.
type DailyQuote struct {
	Close         *float64
	Change        *float64
	ChangePercent *float64
}

// DailyQuoteReader reads the latest visible daily quotes for instrument row IDs.
type DailyQuoteReader interface {
	LatestDailyQuotes(ctx context.Context, ids []uint64) (map[uint64]DailyQuote, error)
}
