package mysql

import "time"

type SignalSnapshotModel struct {
	BaseModel
	SnapshotID      string    `gorm:"size:64;not null;uniqueIndex:uq_snapshot_id"`
	RunID           string    `gorm:"size:64;not null;uniqueIndex:uq_snapshot_run"`
	StrategyID      string    `gorm:"size:64;not null;uniqueIndex:uq_snapshot_business,priority:1;index:idx_snapshot_latest,priority:1"`
	StrategyVersion string    `gorm:"size:32;not null;uniqueIndex:uq_snapshot_business,priority:2;index:idx_snapshot_latest,priority:2"`
	ParametersHash  string    `gorm:"size:64;not null;uniqueIndex:uq_snapshot_business,priority:3;index:idx_snapshot_latest,priority:3"`
	DataVersion     uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_snapshot_business,priority:4"`
	AsOf            time.Time `gorm:"type:datetime(6);not null;uniqueIndex:uq_snapshot_business,priority:5;index:idx_snapshot_latest,priority:5"`
	Status          string    `gorm:"size:32;not null;index:idx_snapshot_latest,priority:4"`
	SuccessCount    uint64    `gorm:"type:bigint unsigned;not null"`
	FailureCount    uint64    `gorm:"type:bigint unsigned;not null"`
	FailuresJSON    []byte    `gorm:"type:json;not null"`
}

func (SignalSnapshotModel) TableName() string { return "t_signal_snapshots" }
