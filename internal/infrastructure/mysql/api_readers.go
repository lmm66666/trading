package mysql

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"trading/internal/market"
	"trading/internal/port"
)

var _ port.InstrumentCodeReader = (*MarketDataRepository)(nil)
var _ port.PublishedSnapshotKeyReader = (*SignalSnapshotStore)(nil)

func (r *MarketDataRepository) ResolveCode(ctx context.Context, code string) ([]market.InstrumentID, error) {
	if (market.InstrumentID{Exchange: market.SSE, Code: code}).Validate() != nil {
		return nil, invalid("code must be exactly six digits")
	}
	var models []InstrumentModel
	if err := r.db.WithContext(ctx).Where("code = ? AND active = ?", code, true).Order("exchange").Limit(2).Find(&models).Error; err != nil {
		return nil, err
	}
	ids := make([]market.InstrumentID, 0, len(models))
	for _, model := range models {
		ids = append(ids, market.InstrumentID{Exchange: market.Exchange(model.Exchange), Code: model.Code})
	}
	return ids, nil
}

func (s *SignalSnapshotStore) LatestPublishedKey(ctx context.Context, strategyID, version, snapshotID string) (port.SnapshotKey, error) {
	for _, f := range []struct {
		v        string
		limit    int
		optional bool
	}{{strategyID, port.MaxStrategyIDBytes, false}, {version, port.MaxStrategyVersionBytes, true}, {snapshotID, port.MaxSnapshotIDBytes, true}} {
		if err := port.ValidateIdentity(f.v, "snapshot selector", f.limit, f.optional); err != nil {
			return port.SnapshotKey{}, err
		}
	}
	query := s.db.WithContext(ctx).Where("strategy_id = ? AND status IN ?", strategyID, []string{string(port.RunSucceeded), string(port.RunPartialSucceeded)})
	if version != "" {
		query = query.Where("strategy_version = ?", version)
	}
	if snapshotID != "" {
		query = query.Where("snapshot_id = ?", snapshotID)
	}
	var model SignalSnapshotModel
	if err := query.Order("as_of DESC, data_version DESC, id DESC").Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return port.SnapshotKey{}, port.ErrSnapshotNotReady
		}
		return port.SnapshotKey{}, err
	}
	key := port.SnapshotKey{SnapshotID: model.SnapshotID, StrategyID: model.StrategyID, StrategyVersion: model.StrategyVersion, ParametersHash: model.ParametersHash, AsOf: model.AsOf.UTC()}
	return key, key.Validate()
}
