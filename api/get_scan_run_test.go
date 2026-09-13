package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestGetScanRunSupportsPartialSuccess(t *testing.T) {
	f := newKernelFixture(t)
	f.store.run.Kind = port.RunScan
	f.store.run.Status = port.RunPartialSucceeded
	w := kernelRequest(t, f, "GET", "/api/v1/scan-runs/run-1", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `PARTIAL_SUCCEEDED`)
	require.NotContains(t, w.Body.String(), "secret")
}
