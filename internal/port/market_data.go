package port

import (
	"context"
	"fmt"
	"time"

	"trading/internal/market"
)

type MarketData interface {
	LatestCompleteVersion(ctx context.Context) (market.DataVersion, error)
	Dataset(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time, version market.DataVersion) (market.Dataset, []market.AdjustmentFactor, []market.CorporateAction, error)
	BatchDatasets(ctx context.Context, ids []market.InstrumentID, request BatchRequest) (map[market.InstrumentID]Bundle, map[market.InstrumentID]error)
	Instruments(ctx context.Context, scope InstrumentScope) ([]market.InstrumentID, error)
	DirtyInstruments(ctx context.Context, after, through market.DataVersion) ([]market.InstrumentID, error)
}

type DataQuality string

const (
	DataComplete   DataQuality = "COMPLETE"
	DataIncomplete DataQuality = "INCOMPLETE"
)

func (quality DataQuality) Validate() error {
	if quality != DataComplete && quality != DataIncomplete {
		return invalidPortValue("unknown data quality %q", quality)
	}
	return nil
}

// InstrumentScope bounds one scan's instrument set. Empty Exchanges means all
// supported exchanges; Limit is always explicit so a scan cannot be unbounded.
type InstrumentScope struct {
	Exchanges  []market.Exchange `json:"exchanges,omitempty"`
	ActiveOnly bool              `json:"active_only"`
	Limit      int               `json:"limit"`
}

func (scope InstrumentScope) Validate() error {
	if scope.Limit < 1 || scope.Limit > MaxScanInstruments {
		return invalidPortValue("instrument limit must be between 1 and %d", MaxScanInstruments)
	}
	seen := make(map[market.Exchange]struct{}, len(scope.Exchanges))
	for _, exchange := range scope.Exchanges {
		if !validExchange(exchange) {
			return invalidPortValue("invalid exchange %q", exchange)
		}
		if _, duplicate := seen[exchange]; duplicate {
			return invalidPortValue("duplicate exchange %q", exchange)
		}
		seen[exchange] = struct{}{}
	}
	return nil
}

// BatchRequest identifies an immutable, versioned market-data window. Its
// Auxiliary slice remains owned by its caller and Validate never reorders it.
type BatchRequest struct {
	PrimaryTimeframe market.Timeframe   `json:"primary_timeframe"`
	Auxiliary        []market.Timeframe `json:"auxiliary,omitempty"`
	From             time.Time          `json:"from"`
	To               time.Time          `json:"to"`
	Version          market.DataVersion `json:"version"`
	LookbackBars     int                `json:"lookback_bars"`
}

func (request BatchRequest) Validate() error {
	if !request.PrimaryTimeframe.Valid() {
		return invalidPortValue("primary timeframe is invalid")
	}
	if err := validateUTCTime(request.From, "from", false); err != nil {
		return err
	}
	if err := validateUTCTime(request.To, "to", false); err != nil {
		return err
	}
	if request.To.Before(request.From) {
		return invalidPortValue("from must not be after to")
	}
	if request.To.After(request.From.AddDate(MaxBacktestRangeYears, 0, 0)) {
		return invalidPortValue("date range exceeds %d years", MaxBacktestRangeYears)
	}
	if request.Version == 0 {
		return invalidPortValue("data version is required")
	}
	if request.LookbackBars < 0 || request.LookbackBars > MaxLookbackBars {
		return invalidPortValue("lookback bars must be between 0 and %d", MaxLookbackBars)
	}
	seen := make(map[market.Timeframe]struct{}, len(request.Auxiliary))
	for _, timeframe := range request.Auxiliary {
		if !timeframe.Valid() {
			return invalidPortValue("auxiliary timeframe is invalid")
		}
		if timeframe == request.PrimaryTimeframe {
			return invalidPortValue("primary timeframe cannot be auxiliary")
		}
		if _, duplicate := seen[timeframe]; duplicate {
			return invalidPortValue("duplicate auxiliary timeframe %d", timeframe)
		}
		seen[timeframe] = struct{}{}
	}
	return nil
}

// Bundle is one instrument's version-pinned datasets. Its maps and slices are
// response-owned: implementations return fresh values and callers may mutate
// only their own returned copy.
type Bundle struct {
	Primary   market.Dataset                      `json:"primary"`
	Auxiliary map[market.Timeframe]market.Dataset `json:"auxiliary,omitempty"`
	Factors   []market.AdjustmentFactor           `json:"factors,omitempty"`
	Actions   []market.CorporateAction            `json:"actions,omitempty"`
	Quality   DataQuality                         `json:"quality"`
}

func (bundle Bundle) Validate() error {
	if err := bundle.Quality.Validate(); err != nil {
		return err
	}
	for timeframe := range bundle.Auxiliary {
		if !timeframe.Valid() {
			return invalidPortValue("invalid auxiliary dataset timeframe %d", timeframe)
		}
	}
	return nil
}

func validExchange(exchange market.Exchange) bool {
	switch exchange {
	case market.SSE, market.SZSE, market.BSE, market.SHFE, market.INE, market.DCE, market.CZCE:
		return true
	default:
		return false
	}
}

func validateInstrument(id market.InstrumentID, field string) error {
	if err := id.Validate(); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidPortValue, field, err)
	}
	return nil
}
