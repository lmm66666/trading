package mysql

import "time"

type AdjustmentFactorModel struct {
	BaseModel
	InstrumentID     uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_factor_version,priority:1;index:idx_factor_dirty,priority:2"`
	EffectiveTime    time.Time `gorm:"type:datetime(6);not null;uniqueIndex:uq_factor_version,priority:2"`
	DataVersion      uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_factor_version,priority:3"`
	Numerator        int64     `gorm:"type:bigint;not null"`
	Denominator      int64     `gorm:"type:bigint;not null"`
	ValidFromVersion uint64    `gorm:"type:bigint unsigned;not null;index:idx_factor_dirty,priority:1"`
	ValidToVersion   *uint64   `gorm:"type:bigint unsigned;index:idx_factor_valid_to"`
}

func (AdjustmentFactorModel) TableName() string { return "t_adjustment_factors" }
