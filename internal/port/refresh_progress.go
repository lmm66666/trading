package port

import (
	"context"
	"errors"
	"time"
	"trading/internal/market"
)

var ErrRefreshRunNotFound = errors.New("refresh run not found")

type RefreshRun struct {
	ID             uint64     `json:"id"`
	RunID          string     `json:"run_id"`
	Kind           string     `json:"kind"`
	Trigger        string     `json:"trigger"`
	State          string     `json:"state"`
	Total          *int       `json:"total"`
	Succeeded      int        `json:"succeeded"`
	Failed         int        `json:"failed"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	HeartbeatAt    time.Time  `json:"heartbeat_at"`
	LastProgressAt *time.Time `json:"last_progress_at"`
	SnapshotAt     time.Time  `json:"snapshot_at"`
	Revision       uint64     `json:"snapshot_revision"`
	ErrorCode      string     `json:"error_code,omitempty"`
}

func (r RefreshRun) Active() bool { return r.State == "PREPARING" || r.State == "RUNNING" }
func (r RefreshRun) Validate() error {
	if ValidateIdentity(r.RunID, "run_id", 64, false) != nil || (r.Kind != "STOCK" && r.Kind != "FUTURES") || (r.Trigger != "MANUAL" && r.Trigger != "SCHEDULED") || r.Revision == 0 || r.Succeeded < 0 || r.Failed < 0 {
		return ErrInvalidPortValue
	}
	switch r.State {
	case "PREPARING", "RUNNING", "SUCCEEDED", "PARTIAL_SUCCEEDED", "FAILED", "INTERRUPTED":
	default:
		return ErrInvalidPortValue
	}
	if r.Total != nil && (*r.Total < 0 || *r.Total > MaxScanInstruments || r.Succeeded+r.Failed > *r.Total) {
		return ErrInvalidPortValue
	}
	if r.Total == nil && (r.Succeeded+r.Failed != 0 || r.State == "RUNNING") {
		return ErrInvalidPortValue
	}
	if r.Active() != (r.FinishedAt == nil) || r.StartedAt.IsZero() || r.HeartbeatAt.IsZero() || r.SnapshotAt.IsZero() {
		return ErrInvalidPortValue
	}
	if r.ErrorCode != "" && r.ErrorCode != "REFRESH_FAILED" && r.ErrorCode != "INTERRUPTED" {
		return ErrInvalidPortValue
	}
	return nil
}

type RefreshFailure struct {
	ID          uint64          `json:"id"`
	RunID       string          `json:"run_id"`
	Exchange    market.Exchange `json:"exchange"`
	Code        string          `json:"code"`
	Name        string          `json:"name,omitempty"`
	ErrorCode   string          `json:"error_code"`
	CompletedAt time.Time       `json:"completed_at"`
}
type RefreshSnapshot struct {
	Run      RefreshRun
	Failures []RefreshFailure
}

func (s RefreshSnapshot) Validate() error {
	if err := s.Run.Validate(); err != nil {
		return err
	}
	if len(s.Failures) != s.Run.Failed {
		return ErrInvalidPortValue
	}
	seen := map[market.InstrumentID]bool{}
	for _, f := range s.Failures {
		id := market.InstrumentID{Exchange: f.Exchange, Code: f.Code}
		if f.RunID != s.Run.RunID || id.Validate() != nil || seen[id] || f.ErrorCode != "REFRESH_FAILED" || f.CompletedAt.IsZero() {
			return ErrInvalidPortValue
		}
		seen[id] = true
	}
	return nil
}

type RefreshReceipt struct {
	Status            string `json:"status"`
	RunID             string `json:"run_id"`
	ProgressAvailable bool   `json:"progress_available"`
}
type RefreshProgressWriter interface {
	SaveRefresh(context.Context, RefreshSnapshot) error
}
type RefreshProgressReader interface {
	LatestRefreshRun(context.Context, string) (RefreshRun, error)
	ListRefreshRuns(context.Context, string, uint64, int) ([]RefreshRun, error)
	GetRefreshRun(context.Context, string) (RefreshRun, error)
	ListRefreshFailures(context.Context, string, uint64, int) ([]RefreshFailure, error)
}
