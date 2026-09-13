package api

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStrategyCatalogAndVersion(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/strategies", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "daily_b1_buy")
	w = kernelRequest(t, f, "GET", "/api/v1/strategies/daily_b1_buy?version=1", "")
	require.Equal(t, 200, w.Code)
	w = kernelRequest(t, f, "GET", "/api/v1/strategies/daily_b1_buy?version=unknown", "")
	require.Equal(t, 404, w.Code)
}
