package api

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const validScanBody = `{"strategy":"daily_b1_buy","strategy_version":"1","idempotency_key":"scan-1","from":"2026-01-01T00:00:00Z","as_of":"2026-06-01T00:00:00Z","scope":{"limit":5000}}`

func TestCreateScanRunAccepted(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "POST", "/api/v1/scan-runs", validScanBody)
	require.Equal(t, 202, w.Code, w.Body.String())
	require.Equal(t, 5000, f.s.req.Scope.Limit)
	require.NotNil(t, f.s.ctx)
}
func TestCreateScanRejectsUnboundedScopeAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{}`, validScanBody + ` true`, strings.Replace(validScanBody, `5000`, `5001`, 1), strings.Replace(validScanBody, `"scope":`, `"oops":true,"scope":`, 1), strings.Replace(validScanBody, `2026-06-01`, `2025-06-01`, 1)} {
		f := newKernelFixture(t)
		w := kernelRequest(t, f, "POST", "/api/v1/scan-runs", body)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Zero(t, f.s.creates)
	}
}
