package broker

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUpstream          = errors.New("broker: upstream failure")
	ErrUpstreamTimeout   = errors.New("broker: upstream timeout")
	ErrRequestCanceled   = errors.New("broker: request canceled")
	ErrInvalidRequest    = errors.New("broker: invalid request")
	ErrMalformedResponse = errors.New("broker: malformed upstream response")
	ErrIncompleteData    = errors.New("broker: incomplete market data")

	decimalPattern = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)
)

// UpstreamError carries safe transport metadata. It deliberately excludes the
// request URL and response body so callers can surface it without leaking data.
type UpstreamError struct {
	Kind          error
	Cause         error
	StatusCode    int
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *UpstreamError) Error() string {
	message := safeUpstreamKind(e.Kind)
	if e.StatusCode != 0 {
		message = fmt.Sprintf("%s: status=%d", message, e.StatusCode)
	}
	if e.HasRetryAfter {
		message = fmt.Sprintf("%s: retry_after=%s", message, e.RetryAfter)
	}
	return message
}

func safeUpstreamKind(kind error) string {
	switch {
	case errors.Is(kind, ErrUpstreamTimeout):
		return ErrUpstreamTimeout.Error()
	case errors.Is(kind, ErrRequestCanceled):
		return ErrRequestCanceled.Error()
	default:
		return ErrUpstream.Error()
	}
}

func (e *UpstreamError) Unwrap() error {
	if e.Cause == nil {
		return e.Kind
	}
	return errors.Join(e.Kind, e.Cause)
}

// classifyUpstreamRequestError maps transport failures onto safe error kinds
// shared by every upstream client.
func classifyUpstreamRequestError(err error) error {
	if errors.Is(err, context.Canceled) {
		return upstreamError(ErrRequestCanceled, err, 0, "")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return upstreamError(ErrUpstreamTimeout, err, 0, "")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return upstreamError(ErrUpstreamTimeout, err, 0, "")
	}
	return upstreamError(ErrUpstream, err, 0, "")
}

func upstreamError(kind, cause error, statusCode int, retryAfter string) error {
	duration, ok := parseRetryAfter(retryAfter, time.Now())
	return &UpstreamError{Kind: kind, Cause: cause, StatusCode: statusCode, RetryAfter: duration, HasRetryAfter: ok}
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if isNonNegativeDecimal(value) {
		const maxDuration = time.Duration(1<<63 - 1)
		const maxInt64Seconds = "9223372036854775807"
		digits := strings.TrimLeft(value, "0")
		if digits == "" {
			return 0, true
		}
		if len(digits) > len(maxInt64Seconds) || (len(digits) == len(maxInt64Seconds) && digits > maxInt64Seconds) {
			return maxDuration, true
		}
		seconds, _ := strconv.ParseInt(digits, 10, 64)
		if seconds > int64(maxDuration/time.Second) {
			return maxDuration, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		return 0, true
	}
	return when.Sub(now), true
}

func isNonNegativeDecimal(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

// parseScaledPrice converts a plain decimal string into the kernel fixed-point
// scale; any excess precision is rejected instead of rounded.
func parseScaledPrice(value string, scale int64) (int64, error) {
	ratio, err := parseUpstreamDecimal(value)
	if err != nil {
		return 0, err
	}
	scaled, ok := ratInt64(new(big.Rat).Mul(ratio, big.NewRat(scale, 1)))
	if !ok {
		return 0, errors.New("decimal exceeds fixed-point precision")
	}
	return scaled, nil
}

func parseUpstreamDecimal(value string) (*big.Rat, error) {
	if !decimalPattern.MatchString(value) {
		return nil, errors.New("not a finite decimal")
	}
	ratio, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	return ratio, nil
}

func ratInt64(value *big.Rat) (int64, bool) {
	if !value.IsInt() || !value.Num().IsInt64() {
		return 0, false
	}
	return value.Num().Int64(), true
}
