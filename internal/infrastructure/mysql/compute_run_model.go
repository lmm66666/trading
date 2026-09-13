package mysql

import "time"

type ComputeRunModel struct {
	BaseModel
	RunID             string     `gorm:"type:varbinary(64);not null;uniqueIndex:uq_compute_run"`
	Kind              string     `gorm:"size:16;not null;uniqueIndex:uq_compute_idempotency,priority:1"`
	Status            string     `gorm:"size:32;not null;index:idx_compute_claim,priority:1"`
	IdempotencyKey    string     `gorm:"type:varbinary(128);not null;uniqueIndex:uq_compute_idempotency,priority:2"`
	InputHash         string     `gorm:"type:varbinary(64);not null"`
	StrategyID        string     `gorm:"type:varbinary(64);not null"`
	StrategyVersion   string     `gorm:"type:varbinary(32);not null"`
	EngineVersion     string     `gorm:"type:varbinary(32);not null"`
	DataVersion       uint64     `gorm:"type:bigint unsigned;not null"`
	RequestJSON       []byte     `gorm:"type:json;not null"`
	LeaseOwner        string     `gorm:"type:varbinary(128);not null"`
	LeaseToken        string     `gorm:"type:varbinary(128);not null"`
	LeaseUntil        *time.Time `gorm:"type:datetime(6);index:idx_compute_lease"`
	Attempts          uint32     `gorm:"type:int unsigned;not null"`
	NextAttemptAt     *time.Time `gorm:"type:datetime(6);index:idx_compute_claim,priority:2"`
	CancelRequestedAt *time.Time `gorm:"type:datetime(6)"`
	FailureCode       string     `gorm:"size:64;not null"`
	FailureMessage    string     `gorm:"type:text"`
}

func (ComputeRunModel) TableName() string { return "t_compute_runs" }
