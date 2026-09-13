package mysql

import "time"

type SignalSnapshotRowModel struct {
	BaseModel
	SnapshotID   string    `gorm:"size:64;not null;uniqueIndex:uq_snapshot_instrument,priority:1;uniqueIndex:uq_snapshot_sequence,priority:1"`
	InstrumentID uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_snapshot_instrument,priority:2"`
	Sequence     uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_snapshot_sequence,priority:2"`
	SignalTime   time.Time `gorm:"type:datetime(6);not null"`
	Reason       string    `gorm:"type:text;not null"`
	ValuesJSON   []byte    `gorm:"type:json;not null"`
}

func (SignalSnapshotRowModel) TableName() string { return "t_signal_snapshot_rows" }
