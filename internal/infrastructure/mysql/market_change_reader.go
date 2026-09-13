package mysql

import (
	"context"
	"trading/internal/market"
	"trading/internal/port"
)

var _ port.MarketChangeReader = (*MarketDataRepository)(nil)

// MarketChanges uses a fixed number of reads regardless of the dirty universe.
// COMPLETE versions are immutable, so later publications cannot change these
// bounded revision predicates between reads.
func (r *MarketDataRepository) MarketChanges(ctx context.Context, after, through market.DataVersion) (port.MarketChangeSet, error) {
	ids, err := r.DirtyInstruments(ctx, after, through)
	if err != nil {
		return port.MarketChangeSet{}, err
	}
	var changed bool
	err = r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM t_adjustment_factors WHERE valid_from_version > ? AND valid_from_version <= ?) OR EXISTS(SELECT 1 FROM t_corporate_actions WHERE valid_from_version > ? AND valid_from_version <= ?) AS changed`, uint64(after), uint64(through), uint64(after), uint64(through)).Scan(&changed).Error
	if err != nil {
		return port.MarketChangeSet{}, err
	}
	return port.MarketChangeSet{Dirty: ids, FactorsOrActionsChanged: changed}, nil
}
