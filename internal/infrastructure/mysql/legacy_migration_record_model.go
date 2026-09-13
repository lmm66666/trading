package mysql

import "time"

// 暂存与检查点不参与市场版本可见性。按证券索引允许只加载单证券历史。
type legacyMigrationRecord struct {
	Stage         string    `gorm:"type:varbinary(16);primaryKey;index:idx_legacy_instrument,priority:1;index:idx_legacy_source,priority:1"`
	InstrumentKey string    `gorm:"type:varbinary(16);primaryKey;index:idx_legacy_instrument,priority:2"`
	LegacyID      uint64    `gorm:"type:bigint unsigned;primaryKey;autoIncrement:false;index:idx_legacy_instrument,priority:3;index:idx_legacy_source,priority:2"`
	Payload       []byte    `gorm:"type:longblob;not null"`
	CreatedAt     time.Time `gorm:"type:datetime(6);not null" json:"-"`
	UpdatedAt     time.Time `gorm:"type:datetime(6);not null" json:"-"`
}

func (legacyMigrationRecord) TableName() string { return "t_legacy_kernel_migration" }
