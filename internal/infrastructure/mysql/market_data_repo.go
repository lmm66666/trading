package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"gorm.io/gorm"
	"sort"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

type MarketDataRepository struct{ db *gorm.DB }

var _ port.MarketData = (*MarketDataRepository)(nil)
var _ port.MarketDataWriter = (*MarketDataRepository)(nil)

func NewMarketDataRepository(db *gorm.DB) *MarketDataRepository { return &MarketDataRepository{db: db} }

func (r *MarketDataRepository) LatestCompleteVersion(ctx context.Context) (market.DataVersion, error) {
	var row DataVersionModel
	if err := r.db.WithContext(ctx).Where("version > 0 AND status = ?", versionComplete).Order("version DESC").Take(&row).Error; err != nil {
		return 0, fmt.Errorf("latest complete market version: %w", err)
	}
	return market.DataVersion(row.Version), nil
}

func completeVersion(db *gorm.DB, version market.DataVersion) (DataVersionModel, error) {
	var row DataVersionModel
	if version == 0 {
		return row, invalid("version zero is reserved")
	}
	if err := db.Where("version = ? AND version > 0 AND status = ?", uint64(version), versionComplete).Take(&row).Error; err != nil {
		return row, fmt.Errorf("market version %d is not complete: %w", version, err)
	}
	if err := port.DataQuality(row.Quality).Validate(); err != nil {
		return row, fmt.Errorf("market version quality: %w", err)
	}
	return row, nil
}

// visibleAt uses the half-open interval [from,to), including unchanged rows
// originally published before the requested immutable version.
func visibleAt(db *gorm.DB, version uint64) *gorm.DB {
	return db.Where("valid_from_version <= ? AND (valid_to_version IS NULL OR valid_to_version > ?)", version, version)
}

func (r *MarketDataRepository) Dataset(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time, version market.DataVersion) (market.Dataset, []market.AdjustmentFactor, []market.CorporateAction, error) {
	bundles, errs := r.BatchDatasets(ctx, []market.InstrumentID{id}, port.BatchRequest{PrimaryTimeframe: tf, From: from, To: to, Version: version})
	if err := errs[id]; err != nil {
		return market.Dataset{}, nil, nil, err
	}
	b := bundles[id]
	return b.Primary, b.Factors, b.Actions, nil
}

// BatchDatasets uses six SELECTs plus at most two bounded warmup SELECTs,
// independent of instrument count. Repeatable-read keeps the optimization coherent
// when a publisher commits between SELECTs. No returned slice aliases a model.
func (r *MarketDataRepository) BatchDatasets(ctx context.Context, ids []market.InstrumentID, req port.BatchRequest) (map[market.InstrumentID]port.Bundle, map[market.InstrumentID]error) {
	result := make(map[market.InstrumentID]port.Bundle, len(ids))
	failures := make(map[market.InstrumentID]error)
	failAll := func(err error) {
		for _, id := range ids {
			if failures[id] == nil {
				failures[id] = err
			}
			delete(result, id)
		}
	}
	if len(ids) == 0 {
		return result, failures
	}
	if len(ids) > port.MaxScanInstruments {
		failAll(invalid("too many instruments"))
		return result, failures
	}
	if err := req.Validate(); err != nil {
		failAll(err)
		return result, failures
	}
	keys := make([][]any, 0, len(ids))
	unique := make(map[market.InstrumentID]bool, len(ids))
	for _, id := range ids {
		if unique[id] {
			continue
		}
		unique[id] = true
		if err := id.Validate(); err != nil {
			failures[id] = err
			continue
		}
		keys = append(keys, []any{string(id.Exchange), id.Code})
	}
	if len(keys) == 0 {
		return result, failures
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := completeVersion(tx, req.Version)
		if err != nil {
			return err
		}
		var latest DataVersionModel
		if err := tx.Where("version > 0 AND status = ?", versionComplete).Order("version DESC").Take(&latest).Error; err != nil {
			return err
		}
		scope := func(db *gorm.DB) *gorm.DB {
			if latest.Version == uint64(req.Version) {
				return db.Where("valid_to_version IS NULL AND valid_from_version <= ?", uint64(req.Version))
			}
			return visibleAt(db, uint64(req.Version))
		}
		var instruments []InstrumentModel
		if err := tx.Where("(exchange, code) IN ?", keys).Find(&instruments).Error; err != nil {
			return fmt.Errorf("load instruments: %w", err)
		}
		numeric := make([]uint64, 0, len(instruments))
		domainIDs := make(map[uint64]market.InstrumentID, len(instruments))
		found := make(map[market.InstrumentID]bool, len(instruments))
		for _, row := range instruments {
			id := market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}
			if err := id.Validate(); err != nil {
				return fmt.Errorf("stored instrument: %w", err)
			}
			numeric = append(numeric, row.ID)
			domainIDs[row.ID] = id
			found[id] = true
		}
		for id := range unique {
			if !found[id] && failures[id] == nil {
				failures[id] = fmt.Errorf("instrument %s: %w", id, gorm.ErrRecordNotFound)
			}
		}
		if len(numeric) == 0 {
			return nil
		}
		timeframes := append([]market.Timeframe{req.PrimaryTimeframe}, req.Auxiliary...)
		names := make([]string, 0, len(timeframes))
		for _, tf := range timeframes {
			names = append(names, timeframeName(tf))
		}
		var rows []MarketBarModel
		query := scope(tx).Where("instrument_id IN ? AND timeframe IN ? AND close_time >= ? AND close_time <= ?", numeric, names, req.From, req.To)
		if err := query.Order("instrument_id, timeframe, close_time").Find(&rows).Error; err != nil {
			return fmt.Errorf("load market bars: %w", err)
		}
		for _, query := range lookbackQueries(numeric, req) {
			var warmup []MarketBarModel
			if err := tx.Raw(query.sql, query.args...).Scan(&warmup).Error; err != nil {
				return fmt.Errorf("load warmup bars: %w", err)
			}
			rows = append(rows, warmup...)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].CloseTime.Before(rows[j].CloseTime) })
		grouped := make(map[uint64]map[market.Timeframe][]market.Bar, len(numeric))
		for _, row := range rows {
			id := domainIDs[row.InstrumentID]
			bar, err := row.bar(id, req.Version)
			if err != nil {
				failures[id] = fmt.Errorf("decode bar: %w", err)
				continue
			}
			if grouped[row.InstrumentID] == nil {
				grouped[row.InstrumentID] = make(map[market.Timeframe][]market.Bar)
			}
			grouped[row.InstrumentID][bar.Timeframe] = append(grouped[row.InstrumentID][bar.Timeframe], bar)
		}
		var factors []AdjustmentFactorModel
		if err := scope(tx).Where("instrument_id IN ? AND effective_time <= ?", numeric, req.To).Order("instrument_id, effective_time").Find(&factors).Error; err != nil {
			return fmt.Errorf("load adjustment factors: %w", err)
		}
		factorGroups := make(map[uint64][]market.AdjustmentFactor)
		for _, row := range factors {
			factor := market.AdjustmentFactor{EffectiveTime: row.EffectiveTime.UTC(), Numerator: row.Numerator, Denominator: row.Denominator, Version: req.Version}
			id := domainIDs[row.InstrumentID]
			if err := storedTime(factor.EffectiveTime); err != nil {
				failures[id] = err
				continue
			}
			if factor.Numerator <= 0 || factor.Denominator <= 0 {
				failures[id] = invalid("invalid stored adjustment factor")
				continue
			}
			group := factorGroups[row.InstrumentID]
			if len(group) > 0 && group[len(group)-1].EffectiveTime.Equal(factor.EffectiveTime) {
				failures[id] = invalid("overlapping factor revisions")
				continue
			}
			factorGroups[row.InstrumentID] = append(group, factor)
		}
		var actions []CorporateActionModel
		if err := scope(tx).Where("instrument_id IN ? AND ex_date <= ?", numeric, req.To).Order("instrument_id, ex_date, source_event_id").Find(&actions).Error; err != nil {
			return fmt.Errorf("load corporate actions: %w", err)
		}
		actionGroups := make(map[uint64][]market.CorporateAction)
		seenActions := make(map[uint64]map[string]bool)
		for _, row := range actions {
			id := domainIDs[row.InstrumentID]
			a, err := row.action(id, req.Version)
			if err != nil {
				failures[id] = err
				continue
			}
			if seenActions[row.InstrumentID] == nil {
				seenActions[row.InstrumentID] = make(map[string]bool)
			}
			if seenActions[row.InstrumentID][a.ID] {
				failures[id] = invalid("overlapping action revisions")
				continue
			}
			seenActions[row.InstrumentID][a.ID] = true
			actionGroups[row.InstrumentID] = append(actionGroups[row.InstrumentID], a)
		}
		for _, numericID := range numeric {
			id := domainIDs[numericID]
			if failures[id] != nil {
				continue
			}
			bundle := port.Bundle{Quality: port.DataQuality(version.Quality), Auxiliary: make(map[market.Timeframe]market.Dataset), Factors: append([]market.AdjustmentFactor(nil), factorGroups[numericID]...), Actions: append([]market.CorporateAction(nil), actionGroups[numericID]...)}
			for _, tf := range timeframes {
				bars := windowBars(grouped[numericID][tf], req.From, req.LookbackBars)
				dataset, err := market.NewDataset(id, tf, req.Version, bars)
				if err != nil {
					failures[id] = fmt.Errorf("dataset %s: %w", id, err)
					break
				}
				if tf == req.PrimaryTimeframe {
					bundle.Primary = dataset
				} else {
					bundle.Auxiliary[tf] = dataset
				}
			}
			if failures[id] == nil {
				result[id] = bundle
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		failAll(fmt.Errorf("batch market datasets: %w", err))
	}
	return result, failures
}

func windowBars(bars []market.Bar, from time.Time, lookback int) []market.Bar {
	start := sort.Search(len(bars), func(i int) bool { return !bars[i].CloseTime.Before(from) })
	start -= lookback
	if start < 0 {
		start = 0
	}
	return bars[start:]
}

func (r *MarketDataRepository) Instruments(ctx context.Context, scope port.InstrumentScope) ([]market.InstrumentID, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	query := r.db.WithContext(ctx)
	if len(scope.Exchanges) > 0 {
		query = query.Where("exchange IN ?", scope.Exchanges)
	}
	if scope.ActiveOnly {
		query = query.Where("active = ?", true)
	}
	var rows []InstrumentModel
	if err := query.Order("exchange, code").Limit(scope.Limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list instruments: %w", err)
	}
	ids := make([]market.InstrumentID, 0, len(rows))
	for _, row := range rows {
		id := market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("stored instrument: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *MarketDataRepository) DirtyInstruments(ctx context.Context, after, through market.DataVersion) ([]market.InstrumentID, error) {
	if through == 0 || through < after {
		return nil, invalid("invalid dirty version interval")
	}
	db := r.db.WithContext(ctx)
	if _, err := completeVersion(db, through); err != nil {
		return nil, err
	}
	if after > 0 {
		if _, err := completeVersion(db, after); err != nil {
			return nil, err
		}
	}
	var rows []InstrumentModel
	err := db.Raw(`SELECT i.* FROM t_instruments AS i JOIN (
		SELECT instrument_id FROM t_market_bars WHERE valid_from_version > ? AND valid_from_version <= ?
		UNION SELECT instrument_id FROM t_adjustment_factors WHERE valid_from_version > ? AND valid_from_version <= ?
		UNION SELECT instrument_id FROM t_corporate_actions WHERE valid_from_version > ? AND valid_from_version <= ?
	) AS dirty ON dirty.instrument_id = i.id ORDER BY i.exchange, i.code`, uint64(after), uint64(through), uint64(after), uint64(through), uint64(after), uint64(through)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load dirty instruments: %w", err)
	}
	ids := make([]market.InstrumentID, 0, len(rows))
	for _, row := range rows {
		id := market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}
		if err := id.Validate(); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
