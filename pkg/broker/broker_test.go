package broker

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"trading/internal/market"
)

type timeoutStubError struct{}

func (timeoutStubError) Error() string   { return "stub timeout" }
func (timeoutStubError) Timeout() bool   { return true }
func (timeoutStubError) Temporary() bool { return false }

func TestUpstreamErrorRedactsCauseButKeepsUnwrap(t *testing.T) {
	secret := "https://example.test/path?token=secret"
	cause := &url.Error{Op: "Get", URL: secret, Err: context.DeadlineExceeded}
	err := upstreamError(ErrUpstreamTimeout, cause, http.StatusGatewayTimeout, "3")
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "example.test") {
		t.Fatalf("unsafe upstream error text: %q", err)
	}
	if !errors.Is(err, ErrUpstreamTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unwrap lost error identity: %v", err)
	}
	if unsafe := upstreamError(errors.New("kind token=secret"), cause, 0, ""); strings.Contains(unsafe.Error(), "secret") {
		t.Fatalf("unsafe custom kind text: %q", unsafe)
	}
}

func TestClassifyUpstreamRequestErrorKeepsSafeKinds(t *testing.T) {
	if !errors.Is(classifyUpstreamRequestError(context.Canceled), ErrRequestCanceled) {
		t.Fatal("canceled request lost its kind")
	}
	if !errors.Is(classifyUpstreamRequestError(context.DeadlineExceeded), ErrUpstreamTimeout) {
		t.Fatal("deadline lost its kind")
	}
	if !errors.Is(classifyUpstreamRequestError(timeoutStubError{}), ErrUpstreamTimeout) {
		t.Fatal("network timeout lost its kind")
	}
	if !errors.Is(classifyUpstreamRequestError(errors.New("connection reset")), ErrUpstream) {
		t.Fatal("generic transport failure lost its kind")
	}
}

func TestParseScaledPriceRejectsInvalidAndImpreciseValues(t *testing.T) {
	if value, err := parseScaledPrice("12.3456", market.ValueScale); err != nil || value != 123456 {
		t.Fatalf("parseScaledPrice(12.3456) = %d, %v", value, err)
	}
	if value, err := parseScaledPrice("10", market.ValueScale); err != nil || value != 100000 {
		t.Fatalf("parseScaledPrice(10) = %d, %v", value, err)
	}
	for _, value := range []string{"12.34567", "1,234", "", " 1", "1 ", "abc", "0x10", "1e3", "NaN", "Infinity", ".5", "1."} {
		if _, err := parseScaledPrice(value, market.ValueScale); err == nil {
			t.Fatalf("parseScaledPrice(%q) unexpectedly succeeded", value)
		}
	}
	if _, ok := ratInt64(big.NewRat(1, 2)); ok {
		t.Fatal("non-integer ratio must not convert")
	}
	if value, ok := ratInt64(big.NewRat(24, 1)); !ok || value != 24 {
		t.Fatalf("ratInt64(24) = %d, %t", value, ok)
	}
}

func TestParseRetryAfterSaturatesDecimalBeyondInt64(t *testing.T) {
	duration, ok := parseRetryAfter(strings.Repeat("9", 100_000), time.Now())
	if !ok || duration != time.Duration(1<<63-1) {
		t.Fatalf("oversized Retry-After = %s, %t", duration, ok)
	}
}

func TestParseRetryAfterStripsLeadingZeroesBeforeRangeCheck(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{value: strings.Repeat("0", 100_000) + "1", want: time.Second},
		{value: strings.Repeat("0", 100_000), want: 0},
		{value: strings.Repeat("0", 100_000) + strings.Repeat("9", 20), want: time.Duration(1<<63 - 1)},
	} {
		duration, ok := parseRetryAfter(tc.value, time.Now())
		if !ok || duration != tc.want {
			t.Fatalf("Retry-After %q = %s, %t; want %s, true", tc.value[:min(len(tc.value), 32)], duration, ok, tc.want)
		}
	}
}

func TestParseRetryAfterNeverOverflowsNegative(t *testing.T) {
	duration, ok := parseRetryAfter("9223372036854775807", time.Now())
	if !ok || duration < 0 {
		t.Fatalf("overflow Retry-After = %s, %t", duration, ok)
	}
}
