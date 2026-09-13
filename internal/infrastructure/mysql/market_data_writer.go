package mysql

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"reflect"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

// Publish applies an incremental observation batch, NOT an authoritative
// snapshot. Missing keys never mean deletion. COMPLETE describes atomic commit
// of supplied observations; upstream quality cannot be inferred from absence of
// company actions/factors. Consumers requiring adjusted prices validate factors.
// A nonempty supplied Digest must equal MarketBatchDigest's canonical SHA-256.
func (r *MarketDataRepository) Publish(ctx context.Context, input port.MarketWriteBatch) (market.DataVersion, error) {
	batch, digest, err := canonicalBatch(input)
	if err != nil {
		return 0, fmt.Errorf("validate market publication: %w", err)
	}
	var published market.DataVersion
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// All publishers acquire the same permanent row before any read. The
		// locking read sees the latest committed state even under MySQL RR.
		var anchor DataVersionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = 0 AND status = ? AND source = ?", versionLock, versionLockSource).Take(&anchor).Error; err != nil {
			return fmt.Errorf("lock market version allocator: %w", err)
		}
		var instrument InstrumentModel
		err := tx.Where("exchange = ? AND code = ?", string(batch.Instrument.Exchange), batch.Instrument.Code).Take(&instrument).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Metadata is unknown until a universe sync supplies it. Never invent
			// a display name, board, active listing status, or trading lot size.
			instrument = InstrumentModel{Exchange: string(batch.Instrument.Exchange), Code: batch.Instrument.Code, Source: batch.Source}
			if err := tx.Create(&instrument).Error; err != nil {
				return fmt.Errorf("create instrument: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("lookup instrument: %w", err)
		}
		var previous DataVersionModel
		err = tx.Where("instrument_id = ? AND version > 0 AND status = ?", instrument.ID, versionComplete).Order("version DESC").Take(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup publication digest: %w", err)
		}
		if err == nil && previous.Digest == digest {
			published = market.DataVersion(previous.Version)
			return nil
		}
		var latest DataVersionModel
		if err := tx.Order("version DESC").Take(&latest).Error; err != nil {
			return fmt.Errorf("allocate market version: %w", err)
		}
		// An initial migration commits bounded batches behind this real,
		// unreadable version. A newer COMPLETE version must not expose them.
		if latest.Source == legacyMigrationSource && latest.Status == string(port.DataIncomplete) {
			return ErrLegacyMigrationBusy
		}
		if latest.Version == math.MaxUint64 {
			return invalid("market version exhausted")
		}
		v := latest.Version + 1
		pending := DataVersionModel{Version: v, InstrumentID: instrument.ID, Source: batch.Source, Status: versionPending, Quality: string(port.DataComplete), Digest: digest}
		if err := tx.Create(&pending).Error; err != nil {
			return fmt.Errorf("create pending version: %w", err)
		}
		if err := publishBars(tx, instrument.ID, batch, v); err != nil {
			return err
		}
		if err := publishFactors(tx, instrument.ID, batch.Factors, v); err != nil {
			return err
		}
		if err := publishActions(tx, instrument.ID, batch, v); err != nil {
			return err
		}
		now := time.Now().UTC()
		update := tx.Model(&DataVersionModel{}).Where("version = ? AND status = ?", v, versionPending).Updates(map[string]any{"status": versionComplete, "published_at": now})
		if update.Error != nil {
			return fmt.Errorf("complete market version: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("pending market version disappeared")
		}
		published = market.DataVersion(v)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("publish market batch: %w", err)
	}
	return published, nil
}

func closeRevisions(tx *gorm.DB, model any, ids []uint64, v uint64) error {
	if len(ids) == 0 {
		return nil
	}
	result := tx.Model(model).Where("id IN ? AND valid_to_version IS NULL", ids).Update("valid_to_version", v)
	if result.Error != nil {
		return fmt.Errorf("close %T revisions: %w", model, result.Error)
	}
	if result.RowsAffected != int64(len(ids)) {
		return fmt.Errorf("concurrent or corrupt %T current revisions", model)
	}
	return nil
}

type barKey struct {
	timeframe string
	closeTime time.Time
}

func barChanges(id uint64, batch port.MarketWriteBatch, current []MarketBarModel, v uint64) ([]uint64, []MarketBarModel, error) {
	byKey := make(map[barKey]MarketBarModel, len(current))
	for _, row := range current {
		key := barKey{row.Timeframe, row.CloseTime.UTC()}
		if _, ok := byKey[key]; ok {
			return nil, nil, invalid("overlapping current bar revisions")
		}
		byKey[key] = row
	}
	var closed []uint64
	var inserted []MarketBarModel
	for _, tf := range []market.Timeframe{market.Day, market.Week, market.Month} {
		for _, bar := range batch.Bars[tf] {
			revision := uint32(1)
			if old, ok := byKey[barKey{timeframeName(tf), bar.CloseTime}]; ok {
				oldBar, err := old.bar(batch.Instrument, 0)
				if err != nil {
					return nil, nil, err
				}
				if reflect.DeepEqual(oldBar, bar) {
					continue
				}
				if old.Revision == math.MaxUint32 {
					return nil, nil, invalid("bar revision exhausted")
				}
				revision = old.Revision + 1
				closed = append(closed, old.ID)
			}
			inserted = append(inserted, barModel(id, bar, revision, v))
		}
	}
	return closed, inserted, nil
}

func publishBars(tx *gorm.DB, id uint64, batch port.MarketWriteBatch, v uint64) error {
	if len(batch.Bars) == 0 {
		return nil
	}
	var current []MarketBarModel
	if err := tx.Where("instrument_id = ? AND valid_to_version IS NULL", id).Find(&current).Error; err != nil {
		return fmt.Errorf("read current bars: %w", err)
	}
	closed, inserted, err := barChanges(id, batch, current, v)
	if err != nil {
		return err
	}
	if err := closeRevisions(tx, &MarketBarModel{}, closed, v); err != nil {
		return err
	}
	if len(inserted) > 0 {
		if err := tx.CreateInBatches(inserted, 500).Error; err != nil {
			return fmt.Errorf("insert bar revisions: %w", err)
		}
	}
	return nil
}

func factorChanges(id uint64, factors []market.AdjustmentFactor, current []AdjustmentFactorModel, v uint64) ([]uint64, []AdjustmentFactorModel, error) {
	byTime := make(map[time.Time]AdjustmentFactorModel, len(current))
	for _, row := range current {
		key := row.EffectiveTime.UTC()
		if _, ok := byTime[key]; ok {
			return nil, nil, invalid("overlapping current factor revisions")
		}
		byTime[key] = row
	}
	var closed []uint64
	var inserted []AdjustmentFactorModel
	for _, f := range factors {
		if old, ok := byTime[f.EffectiveTime]; ok {
			if old.Numerator == f.Numerator && old.Denominator == f.Denominator {
				continue
			}
			closed = append(closed, old.ID)
		}
		inserted = append(inserted, AdjustmentFactorModel{InstrumentID: id, EffectiveTime: f.EffectiveTime, DataVersion: v, Numerator: f.Numerator, Denominator: f.Denominator, ValidFromVersion: v})
	}
	return closed, inserted, nil
}

func publishFactors(tx *gorm.DB, id uint64, factors []market.AdjustmentFactor, v uint64) error {
	if len(factors) == 0 {
		return nil
	}
	var current []AdjustmentFactorModel
	if err := tx.Where("instrument_id = ? AND valid_to_version IS NULL", id).Find(&current).Error; err != nil {
		return fmt.Errorf("read current factors: %w", err)
	}
	closed, inserted, err := factorChanges(id, factors, current, v)
	if err != nil {
		return err
	}
	if err := closeRevisions(tx, &AdjustmentFactorModel{}, closed, v); err != nil {
		return err
	}
	if len(inserted) > 0 {
		if err := tx.CreateInBatches(inserted, 500).Error; err != nil {
			return fmt.Errorf("insert factor revisions: %w", err)
		}
	}
	return nil
}

func actionChanges(id uint64, batch port.MarketWriteBatch, current []CorporateActionModel, v uint64) ([]uint64, []CorporateActionModel, error) {
	byID := make(map[string]CorporateActionModel, len(current))
	for _, row := range current {
		if _, ok := byID[row.SourceEventID]; ok {
			return nil, nil, invalid("overlapping current action revisions")
		}
		byID[row.SourceEventID] = row
	}
	var closed []uint64
	var inserted []CorporateActionModel
	for _, a := range batch.Actions {
		if old, ok := byID[a.ID]; ok {
			oldAction, err := old.action(batch.Instrument, 0)
			if err != nil {
				return nil, nil, err
			}
			if reflect.DeepEqual(oldAction, a) {
				continue
			}
			closed = append(closed, old.ID)
		}
		inserted = append(inserted, actionModel(id, a, v))
	}
	return closed, inserted, nil
}

func publishActions(tx *gorm.DB, id uint64, batch port.MarketWriteBatch, v uint64) error {
	if len(batch.Actions) == 0 {
		return nil
	}
	var current []CorporateActionModel
	if err := tx.Where("instrument_id = ? AND valid_to_version IS NULL", id).Find(&current).Error; err != nil {
		return fmt.Errorf("read current actions: %w", err)
	}
	closed, inserted, err := actionChanges(id, batch, current, v)
	if err != nil {
		return err
	}
	if err := closeRevisions(tx, &CorporateActionModel{}, closed, v); err != nil {
		return err
	}
	if len(inserted) > 0 {
		if err := tx.CreateInBatches(inserted, 500).Error; err != nil {
			return fmt.Errorf("insert action revisions: %w", err)
		}
	}
	return nil
}
