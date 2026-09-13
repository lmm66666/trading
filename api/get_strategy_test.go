package api

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGetStrategyPublishesParameterBounds(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/strategies/daily_b1_buy", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"parameters":`)
	require.Contains(t, w.Body.String(), `"primary_timeframe":"daily"`)
	require.Contains(t, w.Body.String(), `"default_hold_bars":`)
}
