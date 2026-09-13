package api

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const validBacktestBody = `{"instrument":"SSE:600000","strategy":"daily_b1_buy","strategy_version":"1","idempotency_key":"abc","start":"2026-01-01T00:00:00Z","end":"2026-06-01T00:00:00Z","config":{"initial_cash":1000000000,"cash_fraction_bps":10000,"lot_size":100}}`

func TestCreateBacktestRunReturnsAcceptedAndRunID(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "POST", "/api/v1/backtest-runs", validBacktestBody)
	require.Equal(t, 202, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"run_id":"run-1"`)
	require.Equal(t, "SSE:600000", f.b.req.Instrument.String())
	require.NotNil(t, f.b.ctx)
	require.NotContains(t, w.Body.String(), "secret")
}
func TestCreateBacktestRejectsMalformedOrInvalidBodyBeforeCreate(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `[]`, validBacktestBody + ` {}`, strings.Replace(validBacktestBody, `"abc"`, `"abc","unknown":true`, 1), strings.Replace(validBacktestBody, `SSE:600000`, `600000`, 1), strings.Replace(validBacktestBody, `"lot_size":100`, `"lot_size":0`, 1), strings.Replace(validBacktestBody, `"lot_size":100`, `"lot_size":100,"typo":1`, 1), strings.Replace(validBacktestBody, `abc`, strings.Repeat("a", 129), 1), strings.Repeat(" ", 1<<20) + validBacktestBody} {
		t.Run(body[:min(30, len(body))], func(t *testing.T) {
			f := newKernelFixture(t)
			w := kernelRequest(t, f, "POST", "/api/v1/backtest-runs", body)
			require.Equal(t, 400, w.Code, w.Body.String())
			require.Zero(t, f.b.creates)
		})
	}
}
