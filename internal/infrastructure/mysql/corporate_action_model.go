package mysql

import "time"

// The event identity is unique within an instrument and revision. Including the
// version preserves corrections to the same source event instead of overwriting it.
type CorporateActionModel struct {
	BaseModel
	SourceEventID    string    `gorm:"type:varbinary(128);not null;uniqueIndex:uq_action_revision,priority:2"`
	InstrumentID     uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_action_revision,priority:1;index:idx_action_dirty,priority:2;index:idx_action_instrument,priority:1"`
	ExDate           time.Time `gorm:"type:datetime(6);not null;index:idx_action_instrument,priority:2"`
	Kind             string    `gorm:"size:32;not null"`
	CashPerShare     int64     `gorm:"type:bigint;not null"`
	ShareNumerator   int64     `gorm:"type:bigint;not null"`
	ShareDenominator int64     `gorm:"type:bigint;not null"`
	ValidFromVersion uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_action_revision,priority:3;index:idx_action_dirty,priority:1"`
	ValidToVersion   *uint64   `gorm:"type:bigint unsigned;index:idx_action_valid_to"`
}

func (CorporateActionModel) TableName() string { return "t_corporate_actions" }
