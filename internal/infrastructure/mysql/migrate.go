// Package mysql implements the kernel's MySQL 8.4 persistence boundary.
package mysql

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

const (
	versionComplete   = "COMPLETE"
	versionPending    = "PENDING"
	versionLock       = "INTERNAL_LOCK"
	versionLockSource = "__kernel_version_lock__"
)

var migrationModels = []struct {
	model   any
	indexes []string
}{
	{&InstrumentModel{}, []string{"uq_instrument", "idx_instrument_active"}},
	{&DataVersionModel{}, []string{"uq_data_version", "idx_version_instrument"}},
	{&MarketBarModel{}, []string{"uq_bar_revision", "idx_bar_current", "idx_bar_instrument", "idx_bar_dirty", "idx_bar_valid_to"}},
	{&AdjustmentFactorModel{}, []string{"uq_factor_version", "idx_factor_dirty", "idx_factor_valid_to"}},
	{&CorporateActionModel{}, []string{"uq_action_revision", "idx_action_dirty", "idx_action_instrument", "idx_action_valid_to"}},
	{&ComputeRunModel{}, []string{"uq_compute_run", "uq_compute_idempotency", "idx_compute_claim", "idx_compute_lease"}},
	{&BacktestRunModel{}, []string{"uq_backtest_run", "idx_backtest_instrument"}},
	{&BacktestOrderModel{}, []string{"uq_order_sequence"}},
	{&BacktestTradeModel{}, []string{"uq_trade_sequence"}},
	{&BacktestEquityModel{}, []string{"uq_equity_sequence"}},
	{&SignalSnapshotModel{}, []string{"uq_snapshot_id", "uq_snapshot_run", "idx_snapshot_latest"}},
	{&SignalSnapshotRowModel{}, []string{"uq_snapshot_instrument", "uq_snapshot_sequence"}},
	{&OutboxEventModel{}, []string{"uq_outbox_event", "idx_outbox_pending"}},
	{&WatchlistModel{}, []string{"uq_watchlist"}},
}

// Migrate creates tables in dependency order without dropping legacy tables.
// Version zero is a reserved serialization anchor, never a published version.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("migrate kernel: nil database")
	}
	for _, entry := range migrationModels {
		if err := db.AutoMigrate(entry.model); err != nil {
			return fmt.Errorf("migrate %T: %w", entry.model, err)
		}
		for _, name := range entry.indexes {
			if !db.Migrator().HasIndex(entry.model, name) {
				return fmt.Errorf("migrate %T: missing index %s", entry.model, name)
			}
		}
		if _, ok := entry.model.(*SignalSnapshotModel); ok {
			if err := upgradeSnapshotBusinessIndex(db); err != nil {
				return fmt.Errorf("upgrade snapshot index: %w", err)
			}
		}
	}
	now := time.Now().UTC()
	anchor := DataVersionModel{BaseModel: BaseModel{CreatedAt: now, UpdatedAt: now}, Version: 0, Source: versionLockSource, Status: versionLock, Quality: "INTERNAL", Digest: ""}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&anchor).Error; err != nil {
		return fmt.Errorf("initialize version lock: %w", err)
	}
	var stored DataVersionModel
	if err := db.Where("version = 0").Take(&stored).Error; err != nil {
		return fmt.Errorf("verify version lock: %w", err)
	}
	if stored.Status != versionLock || stored.Source != versionLockSource {
		return fmt.Errorf("version zero is not a valid kernel lock")
	}
	return nil
}

// Repeated scans of the same business inputs publish independent snapshots.
// RunID and SnapshotID remain unique; idx_snapshot_latest already indexes the
// query's strategy/parameters/status prefix. AutoMigrate never drops obsolete
// indexes, so explicitly remove only the obsolete business uniqueness rule.
func upgradeSnapshotBusinessIndex(db *gorm.DB) error {
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?", "t_signal_snapshots", "uq_snapshot_business").Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return db.Exec("ALTER TABLE `t_signal_snapshots` DROP INDEX `uq_snapshot_business`").Error
}
