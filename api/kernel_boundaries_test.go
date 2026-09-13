package api

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func TestEveryRunEndpointPropagatesReadFailures(t *testing.T) {
	for _, path := range []string{"/api/v1/backtest-runs/run-1", "/api/v1/scan-runs/run-1", "/api/v1/backtest-runs/run-1/cancel", "/api/v1/scan-runs/run-1/cancel", "/api/v1/backtest-runs/run-1/orders", "/api/v1/backtest-runs/run-1/trades", "/api/v1/backtest-runs/run-1/equity"} {
		f := newKernelFixture(t)
		f.store.err = internalAPIError
		method := "GET"
		if strings.HasSuffix(path, "cancel") {
			method = "POST"
		}
		w := kernelRequest(t, f, method, path, "")
		require.Equal(t, 500, w.Code, path)
		require.NotContains(t, w.Body.String(), "secret")
		require.Zero(t, f.b.cancels)
		require.Zero(t, f.s.cancels)
	}
}
func TestResultAndCancelFailuresRemainRedacted(t *testing.T) {
	for _, path := range []string{"orders", "trades", "equity", "cancel", ""} {
		f := newKernelFixture(t)
		f.store.run.Status = port.RunSucceeded
		f.store.resultErr = internalAPIError
		f.store.pageErr = internalAPIError
		f.b.err = internalAPIError
		method := "GET"
		url := "/api/v1/backtest-runs/run-1"
		if path != "" {
			url += "/" + path
		}
		if path == "cancel" {
			method = "POST"
		}
		w := kernelRequest(t, f, method, url, "")
		require.Equal(t, 500, w.Code, path)
		require.NotContains(t, w.Body.String(), "secret")
	}
	f := newKernelFixture(t)
	f.store.run.Kind = port.RunScan
	f.s.err = internalAPIError
	w := kernelRequest(t, f, "POST", "/api/v1/scan-runs/run-1/cancel", "")
	require.Equal(t, 500, w.Code)
	w = kernelRequest(t, f, "POST", "/api/v1/scan-runs", validScanBody)
	require.Equal(t, 500, w.Code)
}

func TestRunIDAndCatalogIdentitiesAreBounded(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/"+strings.Repeat("a", 65), "")
	require.Equal(t, 400, w.Code)
	w = kernelRequest(t, f, "GET", "/api/v1/strategies/"+strings.Repeat("a", 65), "")
	require.Equal(t, 400, w.Code)
}

func TestHandlersPreserveRequestContext(t *testing.T) {
	type requestKey struct{}
	for _, tt := range []struct {
		method, path, body string
		kind               port.RunKind
	}{{"POST", "/api/v1/backtest-runs", validBacktestBody, port.RunBacktest}, {"POST", "/api/v1/scan-runs", validScanBody, port.RunScan}, {"GET", "/api/v1/backtest-runs/run-1", "", port.RunBacktest}, {"POST", "/api/v1/backtest-runs/run-1/cancel", "", port.RunBacktest}, {"GET", "/api/v1/backtest-runs/run-1/equity", "", port.RunBacktest}, {"GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy", "", port.RunScan}} {
		f := newKernelFixture(t)
		f.store.run.Kind = tt.kind
		f.store.run.Status = port.RunSucceeded
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "request-only"))
		defer cancel()
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)).WithContext(ctx)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
		require.Less(t, w.Code, 300, w.Body.String())
		var forwarded context.Context
		switch {
		case f.b.ctx != nil:
			forwarded = f.b.ctx
		case f.s.ctx != nil:
			forwarded = f.s.ctx
		default:
			forwarded = f.store.ctx
		}
		require.Equal(t, "request-only", forwarded.Value(requestKey{}))
		cancel()
		require.ErrorIs(t, forwarded.Err(), context.Canceled)
	}
}

func TestSnapshotSelectorsAndFailures(t *testing.T) {
	for _, tt := range []struct {
		query  string
		status int
	}{{"limit=1001", 400}, {"strategy=daily_b1_buy&as_of=bad", 400}, {"strategy=daily_b1_buy&parameters_hash=other", 409}, {"strategy=daily_b1_buy&strategy_version=1&parameters_hash=hash-1&as_of=2026-01-01T08:00:00%2B08:00", 200}} {
		f := newKernelFixture(t)
		w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?"+tt.query, "")
		require.Equal(t, tt.status, w.Code, w.Body.String())
		if tt.status == 200 {
			require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), f.s.key.AsOf)
		}
	}
	f := newKernelFixture(t)
	f.lookup.err = internalAPIError
	w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy", "")
	require.Equal(t, 500, w.Code)
	f.lookup.err = nil
	f.s.err = port.ErrSnapshotNotReady
	w = kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy", "")
	require.Equal(t, 409, w.Code)
}

func TestSnapshotContinuationRemainsPinnedWhenNewSnapshotPublishes(t *testing.T) {
	f := newKernelFixture(t)
	f.s.latest = func(_ context.Context, key port.SnapshotKey, page port.PageRequest) (port.SignalSnapshot, error) {
		require.Equal(t, "snap-1", key.SnapshotID)
		snap := f.s.snapshot
		if page.AfterSequence == 1 {
			snap.Rows = nil
		}
		return snap, nil
	}
	w := kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&limit=1", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"next_sequence":1`)
	f.lookup.key.SnapshotID = "newer-snapshot"
	w = kernelRequest(t, f, "GET", "/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&strategy_version=1&parameters_hash=hash-1&snapshot_id=snap-1&limit=1&after_sequence=1", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"rows":[]`)
	require.NotContains(t, w.Body.String(), "next_sequence")
}

func TestLegacySignalLoadsAllPagesOfOneSnapshot(t *testing.T) {
	f := newKernelFixture(t)
	seen := 0
	f.s.latest = func(_ context.Context, key port.SnapshotKey, page port.PageRequest) (port.SignalSnapshot, error) {
		require.Equal(t, "snap-1", key.SnapshotID)
		seen++
		snapshot := f.s.snapshot
		if page.AfterSequence == 0 {
			snapshot.Rows = make([]port.SnapshotRow, 1000)
			for i := range snapshot.Rows {
				snapshot.Rows[i].Instrument = market.InstrumentID{Exchange: market.SSE, Code: "600000"}
			}
		} else {
			require.Equal(t, int64(1000), page.AfterSequence)
		}
		return snapshot, nil
	}
	w := kernelRequest(t, f, "GET", "/api/stocks/signal?strategy=daily_b1_buy", "")
	require.Equal(t, 200, w.Code)
	require.Equal(t, 2, seen)
	require.Equal(t, 1001, strings.Count(w.Body.String(), `"600000"`))
}

func TestLegacyWaitObservesCancellationAndFailures(t *testing.T) {
	f := newKernelFixture(t)
	h := &StockHandler{kernel: f.services}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.waitBacktest(ctx, "run-1")
	require.ErrorIs(t, err, context.Canceled)
	f.store.run.Status = port.RunFailed
	w := kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 409, w.Code)
	f.store.run.Status = port.RunSucceeded
	f.store.resultErr = internalAPIError
	w = kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "secret")
	f.store.resultErr = nil
	f.store.pageErr = internalAPIError
	w = kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 500, w.Code)
	f.lookup.err = port.ErrMarketDataNotFound
	w = kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 404, w.Code)
}

func TestSynchronousResultPagesRejectStalledCursorAndRespectCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readAllResults(ctx, "run", func(context.Context, string, port.PageRequest) (port.Page[int], error) {
		t.Fatal("cancelled read called store")
		return port.Page[int]{}, nil
	})
	require.ErrorIs(t, err, context.Canceled)
	_, err = readAllResults(context.Background(), "run", func(context.Context, string, port.PageRequest) (port.Page[int], error) {
		n := int64(0)
		return port.Page[int]{Items: []int{1}, NextSequence: &n}, nil
	})
	require.Error(t, err)
}
