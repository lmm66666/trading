package api

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestCancelBacktestAccepted(t *testing.T) {
	f := newKernelFixture(t)
	w := kernelRequest(t, f, "POST", "/api/v1/backtest-runs/run-1/cancel", "")
	require.Equal(t, 202, w.Code)
	require.Equal(t, 1, f.b.cancels)
	require.NotNil(t, f.b.ctx)
}

func TestCancellationRejectsUnexpectedBodyBeforeMutation(t *testing.T) {
	for _, suffix := range []string{"backtest-runs", "scan-runs"} {
		for _, body := range []string{`{"unknown":true}`, `{} {}`, strings.Repeat(" ", 1<<20) + `{}`} {
			f := newKernelFixture(t)
			w := kernelRequest(t, f, "POST", "/api/v1/"+suffix+"/run-1/cancel", body)
			require.Equal(t, 400, w.Code, w.Body.String())
			require.Zero(t, f.b.cancels)
			require.Zero(t, f.s.cancels)
		}
	}
}
