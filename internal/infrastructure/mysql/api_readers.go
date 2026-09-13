package mysql

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"trading/internal/market"
	"trading/internal/port"
)

var _ port.InstrumentCodeReader = (*MarketDataRepository)(nil)
var _ port.InstrumentCatalog = (*MarketDataRepository)(nil)
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

func (r *MarketDataRepository) Search(ctx context.Context, search port.InstrumentSearch) ([]port.InstrumentSummary, error) {
	query := r.db.WithContext(ctx).Model(&InstrumentModel{}).
		Select("exchange", "code", "name", "board", "active", "lot_size").
		Where("active = ?", true)
	if search.Exchange != "" {
		query = query.Where("exchange = ?", search.Exchange)
	}
	if asciiDigits(search.Query) {
		query = query.Where("code LIKE ?", search.Query+"%").
			Clauses(clause.OrderBy{Expression: clause.Expr{SQL: "CASE WHEN code = ? THEN 0 ELSE 1 END, exchange, code", Vars: []any{search.Query}, WithoutParentheses: true}})
	} else {
		escaped := escapeLike(search.Query)
		query = query.Where("name LIKE ? ESCAPE '\\\\'", "%"+escaped+"%").
			Clauses(clause.OrderBy{Expression: clause.Expr{SQL: "CASE WHEN name = ? THEN 0 WHEN name LIKE ? ESCAPE '\\\\' THEN 1 ELSE 2 END, exchange, code", Vars: []any{search.Query, escaped + "%"}, WithoutParentheses: true}})
	}
	var rows []InstrumentModel
	if err := query.Limit(search.Limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("search instruments: %w", err)
	}
	return instrumentSummaries(rows)
}

func (r *MarketDataRepository) Get(ctx context.Context, id market.InstrumentID) (port.InstrumentSummary, error) {
	var row InstrumentModel
	err := r.db.WithContext(ctx).Model(&InstrumentModel{}).
		Select("exchange", "code", "name", "board", "active", "lot_size").
		Where("exchange = ? AND code = ? AND active = ?", id.Exchange, id.Code, true).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return port.InstrumentSummary{}, fmt.Errorf("%w: instrument", port.ErrMarketDataNotFound)
	}
	if err != nil {
		return port.InstrumentSummary{}, fmt.Errorf("get instrument: %w", err)
	}
	items, err := instrumentSummaries([]InstrumentModel{row})
	if err != nil {
		return port.InstrumentSummary{}, err
	}
	return items[0], nil
}

func instrumentSummaries(rows []InstrumentModel) ([]port.InstrumentSummary, error) {
	items := make([]port.InstrumentSummary, 0, len(rows))
	for _, row := range rows {
		id := market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("stored instrument: %w", err)
		}
		items = append(items, port.InstrumentSummary{ID: id, Name: row.Name, Board: row.Board, Active: row.Active, LotSize: row.LotSize})
	}
	return items, nil
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
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
