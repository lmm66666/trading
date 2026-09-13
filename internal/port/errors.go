// Package port defines stable application boundaries. Implementations must not
// mutate maps or slices supplied by callers and must return independently owned
// maps and slices. Adapters may use Clone helpers at their persistence boundary.
package port

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrTemporary        = errors.New("temporary infrastructure failure")
	ErrSnapshotNotReady = errors.New("signal snapshot not ready")
	ErrRunNotFound      = errors.New("run not found")
	ErrLeaseLost        = errors.New("run lease lost")
	ErrInvalidPortValue = errors.New("invalid port value")
)

const (
	MaxBacktestRangeYears = 20
	MaxScanInstruments    = 5_000
	MaxPageSize           = 1_000
	MaxLookbackBars       = 10_000
)

func invalidPortValue(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidPortValue}, args...)...)
}

func validateUTCTime(value time.Time, field string, allowZero bool) error {
	if value.IsZero() {
		if allowZero {
			return nil
		}
		return invalidPortValue("%s is required", field)
	}
	if value.Location() != time.UTC {
		return invalidPortValue("%s must be UTC", field)
	}
	return nil
}
