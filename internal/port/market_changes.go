package port

import (
	"context"
	"trading/internal/market"
)

// MarketChangeReader classifies immutable revisions in (after, through]. A
// factor or corporate action revision invalidates the whole derived snapshot.
// Keeping this capability separate allows readers without revision metadata to
// remain usable with conservative full scan rebuilding.
type MarketChangeReader interface {
	MarketChanges(context.Context, market.DataVersion, market.DataVersion) (MarketChangeSet, error)
}
type MarketChangeSet struct {
	Dirty                   []market.InstrumentID
	FactorsOrActionsChanged bool
}
