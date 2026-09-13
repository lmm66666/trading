package api

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/market"
	"trading/internal/port"
)

func TestLegacyBacktestTimeoutReturnsDurableRun(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 202, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"run_id":"run-1"`)
	require.Equal(t, 1, f.b.creates)
	require.Equal(t, "SSE:600000", f.b.req.Instrument.String())
	require.NotEmpty(t, f.b.req.IdempotencyKey)
	require.Equal(t, "600000", f.lookup.code)
}
func TestLegacyBacktestCodeResolutionAndValidation(t *testing.T) {
	for _, tt := range []struct {
		query  string
		ids    []market.InstrumentID
		status int
	}{{"?strategy=daily_b1_buy", nil, 400}, {"?code=600000", nil, 400}, {"?code=abcdef&strategy=daily_b1_buy", nil, 400}, {"?code=600000&strategy=daily_b1_buy", nil, 404}, {"?code=600000&strategy=daily_b1_buy", []market.InstrumentID{{Exchange: market.SSE, Code: "600000"}, {Exchange: market.BSE, Code: "600000"}}, 409}, {"?code=600000&strategy=daily_b1_buy&cycle=weekly", nil, 400}} {
		f := newKernelFixture(t)
		f.lookup.ids = tt.ids
		w := kernelRequest(t, f, "GET", "/api/stocks/backtest"+tt.query, "")
		require.Equal(t, tt.status, w.Code, w.Body.String())
		require.Zero(t, f.b.creates)
	}
}
func TestLegacyBacktestCompletedResult(t *testing.T) {
	f := newKernelFixture(t)
	f.store.run.Status = port.RunSucceeded
	w := kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"closed_trades":2`)
	require.Contains(t, w.Body.String(), `"code":"600000"`)
	require.Contains(t, w.Body.String(), `"equity":`)
	require.NotContains(t, w.Body.String(), "secret")
}
func TestLegacyBacktestCreateFailureIsRedacted(t *testing.T) {
	f := newKernelFixture(t)
	f.b.err = internalAPIError
	w := kernelRequest(t, f, "GET", "/api/stocks/backtest?code=600000&strategy=daily_b1_buy", "")
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "secret")
}
