package port

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
)

func TestRefreshSnapshotValidation(t *testing.T) {
	now := time.Now().UTC()
	total := 2
	valid := func() RefreshSnapshot {
		return RefreshSnapshot{Run: RefreshRun{RunID: "run-1", Kind: "STOCK", Trigger: "MANUAL", State: "RUNNING", Total: &total, StartedAt: now, HeartbeatAt: now, SnapshotAt: now, Revision: 1}}
	}
	require.NoError(t, valid().Validate())
	changes := []func(*RefreshSnapshot){func(s *RefreshSnapshot) { s.Run.RunID = "" }, func(s *RefreshSnapshot) { s.Run.Kind = "BAD" }, func(s *RefreshSnapshot) { s.Run.Trigger = "BAD" }, func(s *RefreshSnapshot) { s.Run.State = "BAD" }, func(s *RefreshSnapshot) { s.Run.Revision = 0 }, func(s *RefreshSnapshot) { s.Run.Succeeded = -1 }, func(s *RefreshSnapshot) { s.Run.Failed = -1 }, func(s *RefreshSnapshot) { n := 5001; s.Run.Total = &n }, func(s *RefreshSnapshot) { s.Run.Succeeded = 3 }, func(s *RefreshSnapshot) { s.Run.Total = nil }, func(s *RefreshSnapshot) { s.Run.FinishedAt = &now }, func(s *RefreshSnapshot) { s.Run.StartedAt = time.Time{} }, func(s *RefreshSnapshot) { s.Run.ErrorCode = "secret" }, func(s *RefreshSnapshot) { s.Run.Failed = 1 }}
	for _, change := range changes {
		s := valid()
		change(&s)
		require.ErrorIs(t, s.Validate(), ErrInvalidPortValue)
	}
	s := valid()
	s.Run.Failed = 1
	s.Failures = []RefreshFailure{{RunID: "run-1", Exchange: market.SSE, Code: "600000", ErrorCode: "REFRESH_FAILED", CompletedAt: now}}
	require.NoError(t, s.Validate())
	s.Failures[0].RunID = "other"
	require.ErrorIs(t, s.Validate(), ErrInvalidPortValue)
	s.Failures[0].RunID = "run-1"
	s.Run.Failed = 2
	s.Failures = append(s.Failures, s.Failures[0])
	require.ErrorIs(t, s.Validate(), ErrInvalidPortValue)
}
