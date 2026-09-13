package port_test

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"trading/internal/port"
)

func TestRunIdentityByteLimitsRejectBeforeStorage(t *testing.T) {
	fields := []struct {
		name string
		size int
		set  func(*port.Run, string)
	}{
		{"ID", 64, func(r *port.Run, s string) { r.ID = s }},
		{"IdempotencyKey", 128, func(r *port.Run, s string) { r.IdempotencyKey = s }},
		{"InputHash", 64, func(r *port.Run, s string) { r.InputHash = s }},
		{"StrategyID", 64, func(r *port.Run, s string) { r.StrategyID = s }},
		{"StrategyVersion", 32, func(r *port.Run, s string) { r.StrategyVersion = s }},
		{"EngineVersion", 32, func(r *port.Run, s string) { r.EngineVersion = s }},
		{"LeaseOwner", 128, func(r *port.Run, s string) { r.Status = port.RunRunning; r.LeaseToken = "token"; r.LeaseOwner = s }},
		{"LeaseToken", 128, func(r *port.Run, s string) { r.Status = port.RunRunning; r.LeaseOwner = "owner"; r.LeaseToken = s }},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, s := range []string{strings.Repeat("x", field.size+1), strings.Repeat("中", field.size/3+1), string([]byte{0xff})} {
				r := validRun()
				field.set(&r, s)
				require.ErrorIs(t, r.Validate(), port.ErrInvalidPortValue)
			}
			r := validRun()
			field.set(&r, strings.Repeat("x", field.size))
			require.NoError(t, r.Validate())
			field.set(&r, "Key ")
			require.NoError(t, r.Validate())
		})
	}
}

func TestSnapshotEventAndMarketIdentityLimits(t *testing.T) {
	for _, mutate := range []func(*port.SignalSnapshot){func(s *port.SignalSnapshot) { s.ID = strings.Repeat("x", 65) }, func(s *port.SignalSnapshot) { s.RunID = strings.Repeat("x", 65) }, func(s *port.SignalSnapshot) { s.Key.StrategyID = strings.Repeat("x", 65) }, func(s *port.SignalSnapshot) { s.Key.StrategyVersion = strings.Repeat("x", 33) }, func(s *port.SignalSnapshot) { s.Key.ParametersHash = strings.Repeat("x", 65) }} {
		s := validSnapshot()
		mutate(&s)
		require.ErrorIs(t, s.Validate(), port.ErrInvalidPortValue)
	}
	s := validSnapshot()
	require.NoError(t, (port.Event{ID: "Key ", Kind: "changed", AggregateID: "标识 ", OccurredAt: s.Rows[0].SignalTime}).Validate())
	for _, event := range []port.Event{{ID: strings.Repeat("x", 65), Kind: "changed", AggregateID: "a", OccurredAt: s.Rows[0].SignalTime}, {ID: "event", Kind: "changed", AggregateID: strings.Repeat("中", 43), OccurredAt: s.Rows[0].SignalTime}} {
		require.ErrorIs(t, event.Validate(), port.ErrInvalidPortValue)
	}
	for _, mutate := range []func(*port.MarketWriteBatch){func(b *port.MarketWriteBatch) { b.Source = strings.Repeat("中", 43) }, func(b *port.MarketWriteBatch) { b.Digest = strings.Repeat("x", 65) }} {
		b := validWriteBatch()
		mutate(&b)
		require.ErrorIs(t, b.Validate(), port.ErrInvalidPortValue)
	}
	b := validWriteBatch()
	b.Actions[0].ID = strings.Repeat("中", 43)
	require.ErrorIs(t, b.Validate(), port.ErrInvalidPortValue)
	b.Actions[0].ID = strings.Repeat("x", 128)
	require.NoError(t, b.Validate())
}
