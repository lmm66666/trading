package broker

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrUpstream          = errors.New("broker: upstream failure")
	ErrUpstreamTimeout   = errors.New("broker: upstream timeout")
	ErrRequestCanceled   = errors.New("broker: request canceled")
	ErrInvalidRequest    = errors.New("broker: invalid request")
	ErrMalformedResponse = errors.New("broker: malformed upstream response")
	ErrIncompleteData    = errors.New("broker: incomplete market data")
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
