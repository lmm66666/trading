package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestEquityPreservesIntegerPrecision(t *testing.T) {
	f := newKernelFixture(t)
	f.store.run.Status = port.RunSucceeded
	w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1/equity", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `12345678901234567`)
}
