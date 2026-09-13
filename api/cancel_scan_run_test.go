package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestCancelScanChecksKindBeforeMutating(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "POST", "/api/v1/scan-runs/run-1/cancel", "")
	require.Equal(t, 404, w.Code)
	require.Zero(t, f.s.cancels)
	f.store.run.Kind = port.RunScan
	w = kernelRequest(t, f, "POST", "/api/v1/scan-runs/run-1/cancel", "")
	require.Equal(t, 202, w.Code)
	require.Equal(t, 1, f.s.cancels)
}
