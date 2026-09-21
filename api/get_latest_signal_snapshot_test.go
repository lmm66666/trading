package api

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func TestSnapshotRejectsOverlongSelectorsBeforeLookup(t *testing.T) {
	for _, field := range []string{"strategy_version", "parameters_hash", "snapshot_id"} {
		f := newKernelFixture(t)
		w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&"+field+"="+strings.Repeat("a", 129), "")
		require.Equal(t, 400, w.Code, field)
	}
}
func TestSnapshotAsOfMatchesScanMicrosecondNormalization(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&strategy_version=1&parameters_hash=hash-1&as_of=2026-01-01T00:00:00.123456789Z", "")
	require.Equal(t, 200, w.Code)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC), f.s.key.AsOf)
}

func TestLatestSnapshotReturnsContinuationIdentityAndFailures(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&limit=1", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"snapshot_id":"snap-1"`)
	require.Contains(t, w.Body.String(), `"next_sequence":1`)
	require.Contains(t, w.Body.String(), `SZSE:000001`)
	require.Contains(t, w.Body.String(), `SSE:600000`)
	require.Zero(t, f.s.creates)
}
func TestLatestSnapshotIncludesNamesWhenResolved(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&limit=1", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), `"name"`)
	f.s.snapshot.Rows[0].Name = "浦发银行"
	f.s.snapshot.Failures[market.InstrumentID{Exchange: market.SZSE, Code: "000001"}] = port.Failure{Code: "DATA", Message: "safe", Name: "平安银行"}
	w = kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&limit=1", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"name":"浦发银行"`)
	require.Contains(t, w.Body.String(), `"name":"平安银行"`)
}
func TestLatestSnapshotRequiresIdentityOnContinuation(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&after_sequence=1", "")
	require.Equal(t, 400, w.Code)
	w = kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&after_sequence=1&snapshot_id=snap-1&parameters_hash=hash-1", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "snap-1", f.s.key.SnapshotID)
	require.Equal(t, int64(1), f.s.page.AfterSequence)
}
