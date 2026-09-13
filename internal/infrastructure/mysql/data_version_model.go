package mysql

import "time"

type DataVersionModel struct {
	BaseModel
	Version      uint64     `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_data_version"`
	InstrumentID uint64     `gorm:"type:bigint unsigned;not null;index:idx_version_instrument,priority:1"`
	Source       string     `gorm:"size:128;not null"`
	Status       string     `gorm:"size:16;not null;index:idx_version_instrument,priority:2"`
	Quality      string     `gorm:"size:16;not null"`
	Digest       string     `gorm:"size:64;not null"`
	PublishedAt  *time.Time `gorm:"type:datetime(6)"`
}

func (DataVersionModel) TableName() string { return "t_market_data_versions" }
