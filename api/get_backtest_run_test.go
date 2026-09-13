package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestGetBacktestRunSafeStatusAndSummary(t *testing.T) {
	f := newKernelFixture(t)
	f.store.run.Status = port.RunSucceeded
	w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"closed_trades":2`)
	require.NotContains(t, w.Body.String(), "secret")
	require.NotContains(t, w.Body.String(), "request_json")
}
func TestGetBacktestRejectsScanIdentity(t *testing.T) {
	f := newKernelFixture(t)
	f.store.run.Kind = port.RunScan
	w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1", "")
	require.Equal(t, 404, w.Code)
}
