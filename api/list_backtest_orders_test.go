package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/port"
)

func TestOrdersStableCursorAndReadiness(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1/orders?limit=3&after_sequence=4", "")
	require.Equal(t, 409, w.Code)
	f.store.run.Status = port.RunSucceeded
	w = kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1/orders?limit=3&after_sequence=4", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"next_sequence":7`)
	require.Contains(t, w.Body.String(), `"instrument":"SSE:600000"`)
	require.Equal(t, port.PageRequest{Limit: 3, AfterSequence: 4}, f.store.page)
}
func TestResultPagesRejectInvalidBounds(t *testing.T) {
	for _, path := range []string{"orders", "trades", "equity"} {
		for _, query := range []string{"limit=0", "limit=1001", "limit=-1", "limit=abc", "after_sequence=-1", "after_sequence=9223372036854775808", "limit=2&limit=3"} {
			f := newKernelFixture(t)
			f.store.run.Status = port.RunSucceeded
			w := kernelRequest(t, f, "GET", "/api/v1/backtest-runs/run-1/"+path+"?"+query, "")
			require.Equal(t, 400, w.Code, w.Body.String())
		}
	}
}
