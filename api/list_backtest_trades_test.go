package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestTradesReturnsFillPage(t *testing.T) {
	f := newKernelFixture(t)
	f.store.run.Status = port.RunSucceeded
	w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1/trades?limit=1000", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "fill-1")
	require.Equal(t, 1000, f.store.page.Limit)
}
