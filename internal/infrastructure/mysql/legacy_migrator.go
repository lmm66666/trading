package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

var (
	ErrUnknownExchange      = errors.New("legacy migration: unknown or ambiguous equity code")
	ErrMigrationIncomplete  = errors.New("legacy migration: incomplete market data")
	ErrLegacyChanged        = errors.New("legacy migration: source or checkpoint changed")
	ErrLegacyTargetNotEmpty = errors.New("legacy migration requires an empty initial market version store")
	ErrLegacyMigrationBusy  = errors.New("legacy migration is in progress")
)

const legacyMigrationSource = "legacy-strategy-kernel-v2"

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
type MigrationFailure struct {
	Instrument market.InstrumentID
	Category   string
}
type MigrationReport struct {
	Version              market.DataVersion
	InstrumentCount      int64
	DailyBarCount        int64
	WeeklyBarCount       int64
	RejectedCodes        []string
	Failures             []MigrationFailure
	CompletedInstruments int64
	Quality              port.DataQuality
	Digest               string
	SourceDigest         string
	DailyDates           MigrationDateRange
	WeeklyDates          MigrationDateRange
	LastLegacyIDs        map[string]uint64
	BacktestEnabled      bool
}
type LegacyMigrator struct {
	db     *gorm.DB
	source port.MarketSource
}

func NewLegacyMigrator(db *gorm.DB, source port.MarketSource) *LegacyMigrator {
	return &LegacyMigrator{db: db, source: source}
}

// 迁移连接持有 MySQL 命名锁，进程死亡自动释放。真实 INCOMPLETE
// 版本阻止普通发布器跨过未完成数据；版本0仍只用作原有分配锁。
func (m *LegacyMigrator) Run(ctx context.Context, opts MigrationOptions) (report MigrationReport, err error) {
	defer func() {
		if err != nil {
			report.Quality = port.DataIncomplete
			report.BacktestEnabled = false
		}
	}()
	if m.db == nil || opts.BatchSize < 1 || opts.BatchSize > 10000 {
		return report, invalid("migration requires database and batch size between 1 and 10000")
	}
	db := m.db.WithContext(ctx).Session(&gorm.Session{NowFunc: func() time.Time { return time.Now().UTC() }})
	if opts.DryRun {
		return m.migratePrepared(db, opts, MigrationReport{})
	}
	pool, err := m.db.DB()
	if err != nil {
		return report, err
	}
	if pool.Stats().MaxOpenConnections == 1 {
		return report, invalid("migration requires at least two database connections")
	}
	err = db.Connection(func(conn *gorm.DB) (runErr error) {
		// Connection 返回可变 statement；重建查询会话，防止表名和 LIMIT
		// 从前一条查询泄漏到下一条查询，同时保留同一专用连接。
		conn = conn.Session(&gorm.Session{NewDB: true})
		var acquired int
		defer func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			var released int
			releaseErr := conn.WithContext(cleanup).Raw("SELECT RELEASE_LOCK(SHA2(CONCAT(DATABASE(), ':legacy-kernel'), 256))").Scan(&released).Error
			// sql.Conn.Close 仅归还连接池。无法确认解锁时必须丢弃底层
			// 连接，防止命名锁随池化连接继续存活。
			if releaseErr != nil || (acquired == 1 && released != 1) {
				if dedicated, ok := conn.Statement.ConnPool.(*sql.Conn); ok {
					_ = dedicated.Raw(func(any) error { return driver.ErrBadConn })
				}
			}
			if runErr == nil && (releaseErr != nil || released != 1) {
				runErr = ErrLegacyMigrationBusy
			}
		}()
		if err := conn.Raw("SELECT GET_LOCK(SHA2(CONCAT(DATABASE(), ':legacy-kernel'), 256), 0)").Scan(&acquired).Error; err != nil {
			return err
		}
		if acquired != 1 {
			return ErrLegacyMigrationBusy
		}
		latest, err := inspectLegacyTarget(conn)
		if err != nil {
			return err
		}
		if latest.Version > 0 && latest.Status == versionComplete {
			found, err := readMigrationState(conn, "report", "", 0, &report)
			if err != nil {
				return err
			}
			if !found || report.Version != market.DataVersion(latest.Version) || report.Quality != port.DataComplete {
				return ErrLegacyChanged
			}
			return nil
		}
		if err := conn.AutoMigrate(&legacyMigrationRecord{}); err != nil {
			return err
		}
		previous, err := prepareLegacyMigration(conn)
		if err != nil {
			return err
		}
		report, err = m.migratePrepared(conn, opts, previous)
		return err
	})
	return report, err
}

func (m *LegacyMigrator) migratePrepared(db *gorm.DB, opts MigrationOptions, previous MigrationReport) (MigrationReport, error) {
	// 锁连接保留源快照，池中第二条连接逐批提交检查点。
	checkpointDB := m.db.WithContext(db.Statement.Context).Session(&gorm.Session{NowFunc: func() time.Time { return time.Now().UTC() }})
	ids, report, err := streamLegacySource(db, checkpointDB, opts, previous.Version, previous.SourceDigest != "")
	if err != nil {
		return report, err
	}
	if previous.SourceDigest != "" && previous.SourceDigest != report.SourceDigest {
		return report, ErrLegacyChanged
	}
	persist := func() error {
		if opts.DryRun {
			return nil
		}
		return putMigrationState(db, "report", "", 0, report)
	}
	if err := persist(); err != nil {
		return report, err
	}
	digest := sha256.New()
	fmt.Fprintln(digest, report.SourceDigest)
	for _, id := range ids {
		item, err := loadLegacyInstrument(db, id, opts)
		if err != nil {
			report.Failures = append(report.Failures, MigrationFailure{Instrument: id, Category: "STORAGE_FAILURE"})
			_ = persist()
			return report, err
		}
		var batch port.MarketWriteBatch
		cached := false
		if !opts.DryRun {
			cached, err = readMigrationState(db, "backfill", id.String(), 0, &batch)
			if err != nil {
				report.Failures = append(report.Failures, MigrationFailure{Instrument: id, Category: "STORAGE_FAILURE"})
				_ = persist()
				return report, err
			}
		}
		if cached {
			batch, _, err = canonicalBatch(batch)
			if err == nil && (batch.Instrument != id || batch.Source != legacyMigrationSource) {
				err = ErrLegacyChanged
			}
		} else {
			batch, err = backfillLegacy(db.Statement.Context, m.source, item)
		}
		if err != nil {
			report.Failures = append(report.Failures, MigrationFailure{Instrument: id, Category: legacyFailureCategory(err)})
			if err := persist(); err != nil {
				return report, err
			}
			continue
		}
		if !opts.DryRun {
			if !cached {
				if err := putMigrationState(db, "backfill", id.String(), 0, batch); err != nil {
					report.Failures = append(report.Failures, MigrationFailure{Instrument: id, Category: "STORAGE_FAILURE"})
					_ = persist()
					return report, err
				}
			}
			if err := writeLegacyInstrument(db, report.Version, item, batch, opts.BatchSize); err != nil {
				report.Failures = append(report.Failures, MigrationFailure{Instrument: id, Category: "STORAGE_FAILURE"})
				_ = persist()
				return report, err
			}
		}
		report.CompletedInstruments++
		fmt.Fprintln(digest, batch.Digest)
		report.Digest = hex.EncodeToString(digest.Sum(nil))
		if err := persist(); err != nil {
			return report, err
		}
	}
	if report.Digest == "" {
		report.Digest = hex.EncodeToString(digest.Sum(nil))
	}
	if len(ids) == 0 || len(report.RejectedCodes) > 0 || len(report.Failures) > 0 {
		if err := persist(); err != nil {
			return report, err
		}
		return report, ErrMigrationIncomplete
	}
	if opts.DryRun {
		report.Quality = port.DataComplete
		return report, nil
	}
	if err := completeLegacyMigration(db, &report); err != nil {
		return report, err
	}
	return report, nil
}

func legacyFailureCategory(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "CANCELED"
	case errors.Is(err, ErrMigrationIncomplete):
		return "INCOMPLETE_DATA"
	case errors.Is(err, ErrLegacyChanged), errors.Is(err, port.ErrInvalidPortValue), errors.Is(err, market.ErrInvalidOHLC), errors.Is(err, market.ErrDuplicateBar), errors.Is(err, market.ErrNegativeVolume):
		return "INVALID_DATA"
	default:
		return "SOURCE_UNAVAILABLE"
	}
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
	batch := port.MarketWriteBatch{Source: legacyMigrationSource, Instrument: item.ID, Bars: map[market.Timeframe][]market.Bar{}}
	if source == nil {
		return batch, ErrMigrationIncomplete
	}
	merged := map[time.Time]market.AdjustmentFactor{}
	byTimeframe := map[market.Timeframe]map[time.Time]market.AdjustmentFactor{}
	for _, tf := range []market.Timeframe{market.Day, market.Week} {
		old := item.Bars[tf]
		if len(old) == 0 {
			continue
		}
		dates := map[time.Time]bool{}
		var from, to time.Time
		for _, row := range old {
			bar, err := legacyBar(row, tf)
			if err != nil {
				return batch, err
			}
			if dates[bar.CloseTime] {
				return batch, market.ErrDuplicateBar
			}
			dates[bar.CloseTime] = true
			if from.IsZero() || bar.CloseTime.Before(from) {
				from = bar.CloseTime
			}
			if bar.CloseTime.After(to) {
				to = bar.CloseTime
			}
		}
		bars, factors, err := source.FetchBars(ctx, item.ID, tf, from, to)
		if err != nil {
			return batch, err
		}
		if len(bars) != len(dates) {
			return batch, ErrMigrationIncomplete
		}
		for _, bar := range bars {
			if !dates[bar.CloseTime] {
				return batch, ErrMigrationIncomplete
			}
		}
		own := map[time.Time]market.AdjustmentFactor{}
		for _, factor := range factors {
			if factor.Numerator <= 0 || factor.Denominator <= 0 {
				return batch, ErrMigrationIncomplete
			}
			if _, duplicate := own[factor.EffectiveTime]; duplicate {
				return batch, ErrMigrationIncomplete
			}
			a, b := factor.Numerator, factor.Denominator
			for b != 0 {
				a, b = b, a%b
			}
			factor.Numerator /= a
			factor.Denominator /= a
			factor.Version = 0
			if previous, ok := merged[factor.EffectiveTime]; ok && !sameLegacyFactor(previous, factor) {
				return batch, ErrMigrationIncomplete
			}
			own[factor.EffectiveTime] = factor
			merged[factor.EffectiveTime] = factor
		}
		byTimeframe[tf] = own
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
	for _, factor := range merged {
		batch.Factors = append(batch.Factors, factor)
	}
	// Canonicalization sorts the merged factors exactly once. Validate coverage
	// and cross-timeframe consistency with linear cursors, never per-Bar copies.
	batch, _, err = canonicalBatch(batch)
	if err != nil {
		return batch, err
	}
	for tf, bars := range batch.Bars {
		cursor := 0
		var active, own market.AdjustmentFactor
		for _, bar := range bars {
			for cursor < len(batch.Factors) && !batch.Factors[cursor].EffectiveTime.After(bar.CloseTime) {
				active = batch.Factors[cursor]
				if factor, ok := byTimeframe[tf][active.EffectiveTime]; ok {
					own = factor
				}
				cursor++
			}
			if own.Numerator <= 0 || !sameLegacyFactor(active, own) {
				return batch, ErrMigrationIncomplete
			}
		}
	}
	return batch, nil
}

func sameLegacyFactor(a, b market.AdjustmentFactor) bool {
	return a.Numerator == b.Numerator && a.Denominator == b.Denominator
}
