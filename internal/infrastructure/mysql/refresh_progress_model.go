package mysql

import (
	"time"
	"trading/internal/port"
)

type RefreshRunModel struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement;type:bigint unsigned;index:idx_refresh_kind,priority:2;index:idx_refresh_state,priority:2;index:idx_refresh_latest,priority:3"`
	RunID          string     `gorm:"column:run_id;type:varbinary(64);not null;uniqueIndex:uq_refresh_run"`
	Kind           string     `gorm:"column:kind;size:16;not null;index:idx_refresh_kind,priority:1;index:idx_refresh_latest,priority:1"`
	Trigger        string     `gorm:"column:trigger_source;size:16;not null"`
	State          string     `gorm:"column:state;size:24;not null;index:idx_refresh_state,priority:1"`
	Total          *int       `gorm:"column:total;"`
	Succeeded      int        `gorm:"column:succeeded;not null"`
	Failed         int        `gorm:"column:failed;not null"`
	StartedAt      time.Time  `gorm:"column:started_at;type:datetime(6);not null;index:idx_refresh_latest,priority:2"`
	FinishedAt     *time.Time `gorm:"column:finished_at;type:datetime(6)"`
	HeartbeatAt    time.Time  `gorm:"column:heartbeat_at;type:datetime(6);not null"`
	LastProgressAt *time.Time `gorm:"column:last_progress_at;type:datetime(6)"`
	SnapshotAt     time.Time  `gorm:"column:snapshot_at;type:datetime(6);not null"`
	Revision       uint64     `gorm:"column:revision;type:bigint unsigned;not null"`
	ErrorCode      string     `gorm:"column:error_code;size:32;not null"`
}

func (RefreshRunModel) TableName() string { return "t_market_refresh_runs" }
func refreshRunModel(r port.RefreshRun) RefreshRunModel {
	return RefreshRunModel{ID: r.ID, RunID: r.RunID, Kind: r.Kind, Trigger: r.Trigger, State: r.State, Total: r.Total, Succeeded: r.Succeeded, Failed: r.Failed, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, HeartbeatAt: r.HeartbeatAt, LastProgressAt: r.LastProgressAt, SnapshotAt: r.SnapshotAt, Revision: r.Revision, ErrorCode: r.ErrorCode}
}
func (r RefreshRunModel) dto() port.RefreshRun {
	return port.RefreshRun{ID: r.ID, RunID: r.RunID, Kind: r.Kind, Trigger: r.Trigger, State: r.State, Total: r.Total, Succeeded: r.Succeeded, Failed: r.Failed, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, HeartbeatAt: r.HeartbeatAt, LastProgressAt: r.LastProgressAt, SnapshotAt: r.SnapshotAt, Revision: r.Revision, ErrorCode: r.ErrorCode}
}

const refreshRunColumns = "id,run_id,kind,trigger_source,state,total,succeeded,failed,started_at,finished_at,heartbeat_at,last_progress_at,snapshot_at,revision,error_code"

type RefreshFailureModel struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;type:bigint unsigned;index:idx_refresh_failure_page,priority:2"`
	RunID       string    `gorm:"type:varbinary(64);not null;uniqueIndex:uq_refresh_failure,priority:1;index:idx_refresh_failure_page,priority:1"`
	Exchange    string    `gorm:"size:16;not null;uniqueIndex:uq_refresh_failure,priority:2"`
	Code        string    `gorm:"size:32;not null;uniqueIndex:uq_refresh_failure,priority:3"`
	ErrorCode   string    `gorm:"size:32;not null"`
	CompletedAt time.Time `gorm:"type:datetime(6);not null"`
}

func (RefreshFailureModel) TableName() string { return "t_market_refresh_failures" }
