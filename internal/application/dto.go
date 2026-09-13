// Package application owns use-case DTOs and orchestration services.
package application

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"trading/internal/backtest"
	"trading/internal/market"
)

var (
	ErrInvalidRequest    = errors.New("invalid application request")
	ErrDateRangeTooLarge = errors.New("application request date range exceeds maximum")
)

const MaxBacktestRangeYears = 20

// BacktestRequest contains the complete caller-selected inputs before the
// service resolves a data version. Parameters are caller-owned; Validate does
// not sort, normalize, or otherwise mutate the map.
type BacktestRequest struct {
	Instrument      market.InstrumentID `json:"instrument"`
	StrategyID      string              `json:"strategy_id"`
	StrategyVersion string              `json:"strategy_version"`
	IdempotencyKey  string              `json:"idempotency_key"`
	Parameters      map[string]float64  `json:"parameters,omitempty"`
	Start           time.Time           `json:"start"`
	End             time.Time           `json:"end"`
	Config          backtest.Config     `json:"config"`
}

func (request BacktestRequest) Validate() error {
	if err := request.Instrument.Validate(); err != nil {
		return invalidRequest("instrument: %v", err)
	}
	if strings.TrimSpace(request.StrategyID) == "" || strings.TrimSpace(request.StrategyVersion) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return invalidRequest("strategy ID, strategy version and idempotency key are required")
	}
	if err := validateUTCDate(request.Start, "start"); err != nil {
		return err
	}
	if err := validateUTCDate(request.End, "end"); err != nil {
		return err
	}
	if request.End.Before(request.Start) {
		return invalidRequest("start must not be after end")
	}
	if request.End.After(request.Start.AddDate(MaxBacktestRangeYears, 0, 0)) {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, ErrDateRangeTooLarge)
	}
	for name, value := range request.Parameters {
		if strings.TrimSpace(name) == "" || math.IsNaN(value) || math.IsInf(value, 0) {
			return invalidRequest("parameter is invalid")
		}
	}
	if err := request.Config.Validate(); err != nil {
		return invalidRequest("config: %v", err)
	}
	return nil
}

func invalidRequest(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidRequest}, args...)...)
}

func validateUTCDate(value time.Time, field string) error {
	if value.IsZero() {
		return invalidRequest("%s is required", field)
	}
	if value.Location() != time.UTC {
		return invalidRequest("%s must be UTC", field)
	}
	return nil
}
