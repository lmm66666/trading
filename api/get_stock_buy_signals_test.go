package api

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"trading/internal/port"
)

func TestLegacySignalReadsSnapshotWithoutStartingScan(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/stocks/signal?strategy=daily_b1_buy", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"codes":["600000"]`)
	require.NotContains(t, w.Body.String(), "SSE:")
	require.Zero(t, f.s.creates)
	require.Equal(t, "snap-1", f.s.key.SnapshotID)
}
func TestLegacySignalMissingUnknownAndNotReady(t *testing.T) {
	for _, tt := range []struct {
		query  string
		err    error
		status int
	}{{"", nil, 400}, {"?strategy=missing", nil, 404}, {"?strategy=daily_b1_buy", port.ErrSnapshotNotReady, 409}, {"?strategy=daily_b1_buy", internalAPIError, 500}} {
		f := newKernelFixture(t)
		f.s.err = tt.err
		w := kernelRequest(t, f, "GET", "/api/stocks/signal"+tt.query, "")
		require.Equal(t, tt.status, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "secret")
		require.Zero(t, f.s.creates)
	}
}
func TestLegacySignalEmptyPublishedSnapshot(t *testing.T) {
	f := newKernelFixture(t)
	f.s.snapshot.Rows = nil
	w := kernelRequest(t, f, "GET", "/api/stocks/signal?strategy=daily_b1_buy", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"codes":[]`)
}
func contains(s, substr string) bool { return strings.Contains(s, substr) }
