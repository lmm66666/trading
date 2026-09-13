package mysql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

var (
	ErrUnknownExchange     = errors.New("legacy migration: unknown or ambiguous equity code")
	ErrMigrationIncomplete = errors.New("legacy migration: incomplete market data")
	ErrLegacyChanged       = errors.New("legacy migration: staged source changed; restore the maintenance-window snapshot")
)

const legacyMigrationSource = "legacy-strategy-kernel-v1"

// MapLegacyInstrument deliberately accepts only the documented equity prefixes.
// It does not trim, guess an exchange, or reinterpret exchange-prefixed symbols.
func MapLegacyInstrument(code string) (market.InstrumentID, error) {
	if len(code) != 6 {
		return market.InstrumentID{}, ErrUnknownExchange
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return market.InstrumentID{}, ErrUnknownExchange
		}
	}
	var exchange market.Exchange
	matches := 0
	for ex, prefixes := range map[market.Exchange][]string{market.SSE: {"600", "601", "603", "605", "688", "689"}, market.SZSE: {"000", "001", "002", "003", "300", "301"}, market.BSE: {"4", "8", "920"}} {
		for _, p := range prefixes {
			if strings.HasPrefix(code, p) {
				exchange = ex
				matches++
			}
		}
	}
	if matches != 1 {
		return market.InstrumentID{}, ErrUnknownExchange
	}
	return market.InstrumentID{Exchange: exchange, Code: code}, nil
}

type MigrationOptions struct {
	DryRun    bool
	BatchSize int
}
type MigrationDateRange struct {
	From time.Time
	To   time.Time
}
type MigrationReport struct {
	Version         market.DataVersion
	InstrumentCount int64
	DailyBarCount   int64
	WeeklyBarCount  int64
	RejectedCodes   []string
	Quality         port.DataQuality
	Digest          string
	SourceDigest    string
	DailyDates      MigrationDateRange
	WeeklyDates     MigrationDateRange
	LastLegacyIDs   map[string]uint64
	BacktestEnabled bool
}
type LegacyMigrator struct {
	db     *gorm.DB
	source port.MarketSource
}

func NewLegacyMigrator(db *gorm.DB, source port.MarketSource) *LegacyMigrator {
	return &LegacyMigrator{db: db, source: source}
}
func (m *LegacyMigrator) Run(ctx context.Context, opts MigrationOptions) (MigrationReport, error) {
	if opts.BatchSize < 1 || opts.BatchSize > 10000 || m.db == nil {
		return MigrationReport{}, invalid("migration requires database and batch size between 1 and 10000")
	}
	db := m.db.WithContext(ctx)
	if !opts.DryRun {
		if err := db.AutoMigrate(&legacyMigrationRecord{}); err != nil {
			return MigrationReport{}, err
		}
		if report, found, err := completedLegacyReport(db); err != nil || found {
			return report, err
		}
	}
	records, items, report, err := readLegacy(db, opts.BatchSize)
	if err != nil {
		return report, err
	}
	if !opts.DryRun {
		// Bind every checkpoint (including cached source responses) to the whole
		// legacy snapshot; deletions and insertions below a cursor cannot hide.
		if err := stageLegacy(db, []legacyMigrationRecord{{Stage: "manifest", Payload: []byte(report.SourceDigest)}}, 1); err != nil {
			return report, err
		}
		if err := stageLegacy(db, records, opts.BatchSize); err != nil {
			return report, err
		}
		// Persist the safety state before any external request. Cancellation or
		// process death during backfill leaves an explicitly unreadable version.
		if err := finishLegacy(db, &report, nil, items); err != nil {
			return report, err
		}
	}
	batches := make([]port.MarketWriteBatch, 0, len(items))
	failed := len(report.RejectedCodes) > 0 || len(items) == 0
	for index, item := range items {
		var batch port.MarketWriteBatch
		var cached legacyMigrationRecord
		if !opts.DryRun {
			err = db.Where("stage = ? AND legacy_id = ?", "backfill", uint64(index+1)).Take(&cached).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return report, err
			}
		}
		if len(cached.Payload) > 0 {
			if err = json.Unmarshal(cached.Payload, &batch); err == nil {
				batch, _, err = canonicalBatch(batch)
			}
			if err == nil && batch.Instrument != item.ID {
				err = ErrLegacyChanged
			}
		} else {
			batch, err = backfillLegacy(ctx, m.source, item)
		}
		if err != nil {
			failed = true
			continue
		}
		batches = append(batches, batch)
		if !opts.DryRun && len(cached.Payload) == 0 {
			payload, err := json.Marshal(batch)
			if err != nil {
				return report, err
			}
			if err := stageLegacy(db, []legacyMigrationRecord{{Stage: "backfill", LegacyID: uint64(index + 1), Payload: payload}}, 1); err != nil {
				return report, err
			}
		}
	}
	report.Digest = migrationDigest(report.SourceDigest, batches)
	if !failed {
		report.Quality = port.DataComplete
		report.BacktestEnabled = true
	}
	if !opts.DryRun {
		if err := finishLegacy(db, &report, batches, items); err != nil {
			return report, err
		}
	}
	if failed {
		return report, ErrMigrationIncomplete
	}
	return report, nil
}

// This private staging table never participates in version visibility. Each
// committed (stage, legacy_id) row is a durable checkpoint; replay checks its
// payload instead of overwriting it. No legacy table is modified or deleted.
type legacyMigrationRecord struct {
	Stage     string    `gorm:"type:varbinary(16);primaryKey"`
	LegacyID  uint64    `gorm:"type:bigint unsigned;primaryKey;autoIncrement:false"`
	Payload   []byte    `gorm:"type:longblob;not null"`
	CreatedAt time.Time `gorm:"type:datetime(6);not null" json:"-"`
	UpdatedAt time.Time `gorm:"type:datetime(6);not null" json:"-"`
}

func (legacyMigrationRecord) TableName() string { return "t_legacy_kernel_migration" }

func completedLegacyReport(db *gorm.DB) (MigrationReport, bool, error) {
	var row legacyMigrationRecord
	err := db.Where("stage = ? AND legacy_id = ?", "report", 0).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MigrationReport{}, false, nil
	}
	if err != nil {
		return MigrationReport{}, false, err
	}
	var report MigrationReport
	if err := json.Unmarshal(row.Payload, &report); err != nil {
		return report, false, err
	}
	return report, report.Quality == port.DataComplete, nil
}

func readLegacy(db *gorm.DB, batchSize int) ([]legacyMigrationRecord, []legacyInstrument, MigrationReport, error) {
	report := MigrationReport{Quality: port.DataIncomplete, LastLegacyIDs: map[string]uint64{}, RejectedCodes: []string{}}
	var records []legacyMigrationRecord
	byID := make(map[market.InstrumentID]*legacyInstrument)
	rejected := make(map[string]bool)
	itemFor := func(code string) *legacyInstrument {
		id, err := MapLegacyInstrument(code)
		if err != nil {
			rejected[code] = true
			return nil
		}
		if byID[id] == nil {
			byID[id] = &legacyInstrument{ID: id, Bars: map[market.Timeframe][]model.StockKline{}}
		}
		return byID[id]
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, stage := range []string{"info", "daily", "weekly"} {
			var last uint64
			for {
				var rows []model.StockKline
				var infos []model.StockInfo
				query := tx.Where("id > ?", last).Order("id ASC").Limit(batchSize)
				if stage == "info" {
					if err := query.Find(&infos).Error; err != nil {
						return err
					}
				} else {
					table := "t_stock_kline_daily"
					if stage == "weekly" {
						table = "t_stock_kline_weekly"
					}
					if err := query.Table(table).Find(&rows).Error; err != nil {
						return err
					}
				}
				for _, info := range infos {
					payload, err := json.Marshal(info)
					if err != nil {
						return err
					}
					last = uint64(info.ID)
					records = append(records, legacyMigrationRecord{Stage: stage, LegacyID: last, Payload: payload})
					if item := itemFor(info.Code); item != nil {
						item.Name = info.Name
					}
				}
				for _, row := range rows {
					payload, err := json.Marshal(row)
					if err != nil {
						return err
					}
					last = uint64(row.ID)
					records = append(records, legacyMigrationRecord{Stage: stage, LegacyID: last, Payload: payload})
					item := itemFor(row.Code)
					if item == nil {
						continue
					}
					tf := market.Day
					if stage == "weekly" {
						tf = market.Week
					}
					bar, err := legacyBar(row, tf)
					if err != nil {
						return fmt.Errorf("legacy %s ID %d: %w", stage, row.ID, err)
					}
					item.Bars[tf] = append(item.Bars[tf], row)
					dateRange := &report.DailyDates
					if tf == market.Week {
						dateRange = &report.WeeklyDates
						report.WeeklyBarCount++
					} else {
						report.DailyBarCount++
					}
					if dateRange.From.IsZero() || bar.CloseTime.Before(dateRange.From) {
						dateRange.From = bar.CloseTime
					}
					if bar.CloseTime.After(dateRange.To) {
						dateRange.To = bar.CloseTime
					}
				}
				report.LastLegacyIDs[stage] = last
				if len(infos)+len(rows) < batchSize {
					break
				}
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, nil, report, err
	}
	items := make([]legacyInstrument, 0, len(byID))
	for _, item := range byID {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID.String() < items[j].ID.String() })
	for code := range rejected {
		report.RejectedCodes = append(report.RejectedCodes, code)
	}
	sort.Strings(report.RejectedCodes)
	report.InstrumentCount = int64(len(items))
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for _, row := range records {
		if err := encoder.Encode(row); err != nil {
			return nil, nil, report, err
		}
	}
	report.SourceDigest = hex.EncodeToString(hash.Sum(nil))
	report.Digest = migrationDigest(report.SourceDigest, nil)
	return records, items, report, nil
}

func migrationDigest(sourceDigest string, batches []port.MarketWriteBatch) string {
	hash := sha256.New()
	fmt.Fprintln(hash, sourceDigest)
	for _, b := range batches {
		fmt.Fprintln(hash, b.Digest)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func stageLegacy(db *gorm.DB, records []legacyMigrationRecord, batchSize int) error {
	for start := 0; start < len(records); start += batchSize {
		end := min(start+batchSize, len(records))
		batch := records[start:end]
		if err := db.Transaction(func(tx *gorm.DB) error {
			keys := make([][]any, 0, len(batch))
			for _, row := range batch {
				keys = append(keys, []any{row.Stage, row.LegacyID})
			}
			var existing []legacyMigrationRecord
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("(stage, legacy_id) IN ?", keys).Find(&existing).Error; err != nil {
				return err
			}
			byKey := make(map[string][]byte, len(existing))
			for _, row := range existing {
				byKey[fmt.Sprintf("%s/%d", row.Stage, row.LegacyID)] = row.Payload
			}
			var missing []legacyMigrationRecord
			for _, row := range batch {
				if payload, ok := byKey[fmt.Sprintf("%s/%d", row.Stage, row.LegacyID)]; ok {
					if !bytes.Equal(payload, row.Payload) {
						return ErrLegacyChanged
					}
				} else {
					missing = append(missing, row)
				}
			}
			if len(missing) > 0 {
				now := time.Now().UTC()
				for i := range missing {
					missing[i].CreatedAt = now
					missing[i].UpdatedAt = now
				}
				return tx.CreateInBatches(missing, 500).Error
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func finishLegacy(db *gorm.DB, report *MigrationReport, batches []port.MarketWriteBatch, items []legacyInstrument) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var anchor DataVersionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = 0 AND status = ? AND source = ?", versionLock, versionLockSource).Take(&anchor).Error; err != nil {
			return err
		}
		var previous DataVersionModel
		err := tx.Where("source = ? AND version > 0", legacyMigrationSource).Order("version DESC").Take(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && previous.Status == versionComplete {
			return ErrLegacyChanged
		}
		var latest DataVersionModel
		if err := tx.Order("version DESC").Take(&latest).Error; err != nil {
			return err
		}
		v := previous.Version
		if v == 0 || v < latest.Version {
			if latest.Version == math.MaxUint64 {
				return invalid("market version exhausted")
			}
			v = latest.Version + 1
			row := DataVersionModel{Version: v, Source: legacyMigrationSource, Status: string(port.DataIncomplete), Quality: string(port.DataIncomplete), Digest: report.Digest}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		report.Version = market.DataVersion(v)
		if report.Quality == port.DataComplete {
			for i, batch := range batches {
				var instrument InstrumentModel
				err := tx.Where("exchange = ? AND code = ?", string(batch.Instrument.Exchange), batch.Instrument.Code).Take(&instrument).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					instrument = InstrumentModel{Exchange: string(batch.Instrument.Exchange), Code: batch.Instrument.Code, Name: items[i].Name, Source: legacyMigrationSource}
					if err := tx.Create(&instrument).Error; err != nil {
						return err
					}
				} else if err != nil {
					return err
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
			}
		}
		updates := map[string]any{"status": string(report.Quality), "quality": string(report.Quality), "digest": report.Digest}
		if report.Quality == port.DataComplete {
			updates["published_at"] = time.Now().UTC()
		}
		if err := tx.Model(&DataVersionModel{}).Where("version = ?", v).Updates(updates).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(report)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		row := legacyMigrationRecord{Stage: "report", Payload: payload, CreatedAt: now, UpdatedAt: now}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "stage"}, {Name: "legacy_id"}}, DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"})}).Create(&row).Error
	})
}

type legacyInstrument struct {
	ID   market.InstrumentID
	Name string
	Bars map[market.Timeframe][]model.StockKline
}

func legacyBar(old model.StockKline, tf market.Timeframe) (market.Bar, error) {
	id, err := MapLegacyInstrument(old.Code)
	if err != nil {
		return market.Bar{}, err
	}
	at, err := time.ParseInLocation("2006-01-02", old.Date, time.UTC)
	if err != nil {
		return market.Bar{}, invalid("invalid legacy trading date")
	}
	prices := []float64{old.Open, old.High, old.Low, old.Close}
	scaled := make([]market.Price, 4)
	for i, p := range prices {
		v := math.Round(p * float64(market.ValueScale))
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v >= float64(math.MaxInt64) {
			return market.Bar{}, invalid("legacy price out of range")
		}
		scaled[i] = market.Price(v)
	}
	b := market.Bar{Instrument: id, Timeframe: tf, OpenTime: at, CloseTime: at, Open: scaled[0], High: scaled[1], Low: scaled[2], Close: scaled[3], Volume: old.Volume}
	if old.Volume == 0 {
		b.Trading = market.Suspended
	}
	if _, err := market.NewDataset(id, tf, 0, []market.Bar{b}); err != nil {
		return market.Bar{}, err
	}
	return b, nil
}

// Legacy prices have no reliable adjustment provenance. The source supplies the
// authoritative raw OHLCV; legacy rows define the exact dates that must survive.
func backfillLegacy(ctx context.Context, source port.MarketSource, item legacyInstrument) (port.MarketWriteBatch, error) {
	batch := port.MarketWriteBatch{Source: legacyMigrationSource, Instrument: item.ID, Bars: make(map[market.Timeframe][]market.Bar)}
	if source == nil {
		return batch, ErrMigrationIncomplete
	}
	factorByTime := make(map[time.Time]market.AdjustmentFactor)
	adjustedByTimeframe := make(map[market.Timeframe]map[time.Time]float64)
	for _, tf := range []market.Timeframe{market.Day, market.Week} {
		old := item.Bars[tf]
		if len(old) == 0 {
			continue
		}
		dates := make(map[time.Time]bool, len(old))
		var from, to time.Time
		for _, row := range old {
			b, err := legacyBar(row, tf)
			if err != nil {
				return batch, err
			}
			if dates[b.CloseTime] {
				return batch, market.ErrDuplicateBar
			}
			dates[b.CloseTime] = true
			if from.IsZero() || b.CloseTime.Before(from) {
				from = b.CloseTime
			}
			if b.CloseTime.After(to) {
				to = b.CloseTime
			}
		}
		bars, factors, err := source.FetchBars(ctx, item.ID, tf, from, to)
		if err != nil {
			return batch, err
		}
		if len(bars) != len(dates) {
			return batch, ErrMigrationIncomplete
		}
		// Validate each source series before merging; map insertion must not
		// silently erase duplicate/invalid factors from a malformed provider.
		if _, _, err := canonicalBatch(port.MarketWriteBatch{Source: legacyMigrationSource, Instrument: item.ID, Bars: map[market.Timeframe][]market.Bar{tf: bars}, Factors: factors}); err != nil {
			return batch, err
		}
		adjustedByTimeframe[tf] = make(map[time.Time]float64)
		for _, b := range bars {
			if !dates[b.CloseTime] {
				return batch, ErrMigrationIncomplete
			}
			adjusted, ok := market.AdjustedPrice(b.Close, b.CloseTime, factors)
			if !ok {
				return batch, ErrMigrationIncomplete
			}
			adjustedByTimeframe[tf][b.CloseTime] = adjusted
		}
		for _, f := range factors {
			if previous, ok := factorByTime[f.EffectiveTime]; ok && (previous.Numerator != f.Numerator || previous.Denominator != f.Denominator) {
				return batch, ErrMigrationIncomplete
			}
			f.Version = 0
			factorByTime[f.EffectiveTime] = f
		}
		batch.Bars[tf] = bars
	}
	if len(batch.Bars) == 0 {
		return batch, ErrMigrationIncomplete
	}
	actions, err := source.FetchCorporateActions(ctx, item.ID)
	if err != nil {
		return batch, err
	}
	batch.Actions = actions
	for _, f := range factorByTime {
		batch.Factors = append(batch.Factors, f)
	}
	for tf, bars := range batch.Bars {
		for _, b := range bars {
			adjusted, ok := market.AdjustedPrice(b.Close, b.CloseTime, batch.Factors)
			if !ok || adjusted != adjustedByTimeframe[tf][b.CloseTime] {
				return batch, ErrMigrationIncomplete
			}
		}
	}
	batch, _, err = canonicalBatch(batch)
	return batch, err
}
