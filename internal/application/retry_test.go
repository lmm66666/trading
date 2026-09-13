package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"syscall"
	"testing"
	"time"
	"trading/internal/port"
)

func TestRetryUsesPersistedAttemptsAndTerminalizesFourth(t *testing.T) {
	now := marketDate(1)
	for _, test := range []struct {
		attempt int
		delay   time.Duration
		retry   bool
	}{{0, 0, false}, {1, 250 * time.Millisecond, true}, {2, time.Second, true}, {3, 4 * time.Second, true}, {4, 0, false}, {5, 0, false}} {
		at, ok := retryAt(now, test.attempt)
		require.Equal(t, test.retry, ok)
		if ok {
			require.Equal(t, now.Add(test.delay), at)
		}
	}
}
func TestRetryClassifiesOnlyTransientInfrastructureErrors(t *testing.T) {
	for _, test := range []struct {
		err   error
		retry bool
	}{{fmt.Errorf("wrapped: %w", port.ErrTemporary), true}, {context.DeadlineExceeded, true}, {syscall.ECONNRESET, true}, {syscall.ECONNREFUSED, true}, {context.Canceled, false}, {port.ErrLeaseLost, false}, {ErrIncompleteMarketData, false}, {errors.New("timeout password=secret"), false}} {
		f := classifyFailure(test.err)
		require.Equal(t, test.retry, f.Retryable)
		require.NotContains(t, f.Message, "secret")
	}
}
