package application_test

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"trading/internal/application"
	"trading/internal/backtest"
	"trading/internal/market"
)

func TestBacktestRequestValidateAcceptsCompleteBoundedUTCRequest(t *testing.T) {
	request := validBacktestRequest()
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestBacktestRequestValidateRejectsDateRangeOverTwentyYears(t *testing.T) {
	request := validBacktestRequest()
	request.End = request.Start.AddDate(20, 0, 1)

	if err := request.Validate(); !errors.Is(err, application.ErrDateRangeTooLarge) {
		t.Fatalf("Validate() error = %v, want ErrDateRangeTooLarge", err)
	}
}

func TestBacktestRequestValidateRejectsMalformedRequestFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*application.BacktestRequest)
	}{
		{"instrument", func(request *application.BacktestRequest) { request.Instrument = market.InstrumentID{} }},
		{"strategy ID", func(request *application.BacktestRequest) { request.StrategyID = "" }},
		{"strategy version", func(request *application.BacktestRequest) { request.StrategyVersion = "" }},
		{"idempotency key", func(request *application.BacktestRequest) { request.IdempotencyKey = "" }},
		{"non UTC start", func(request *application.BacktestRequest) {
			request.Start = request.Start.In(time.FixedZone("CST", 8*60*60))
		}},
		{"reversed dates", func(request *application.BacktestRequest) { request.Start, request.End = request.End, request.Start }},
		{"NaN parameter", func(request *application.BacktestRequest) { request.Parameters["threshold"] = math.NaN() }},
		{"infinite parameter", func(request *application.BacktestRequest) { request.Parameters["threshold"] = math.Inf(1) }},
		{"invalid config", func(request *application.BacktestRequest) { request.Config.LotSize = 0 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validBacktestRequest()
			test.mutate(&request)
			if err := request.Validate(); !errors.Is(err, application.ErrInvalidRequest) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestBacktestRequestValidateDoesNotMutateParameters(t *testing.T) {
	request := validBacktestRequest()
	before := mapsClone(request.Parameters)

	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !reflect.DeepEqual(request.Parameters, before) {
		t.Fatalf("Validate() mutated Parameters: got %#v, want %#v", request.Parameters, before)
	}
}

func validBacktestRequest() application.BacktestRequest {
	return application.BacktestRequest{
		Instrument:      market.InstrumentID{Exchange: market.SSE, Code: "600000"},
		StrategyID:      "daily_b1_buy",
		StrategyVersion: "1",
		IdempotencyKey:  "request-1",
		Parameters:      map[string]float64{"threshold": 1.5},
		Start:           time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		End:             time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		Config: backtest.Config{
			InitialCash:       market.Money(100_000 * market.ValueScale),
			CashFractionBPS:   10_000,
			CommissionBPS:     3,
			MinimumCommission: market.Money(5 * market.ValueScale),
			StampDutyBPS:      5,
			TransferFeeBPS:    1,
			LotSize:           100,
		},
	}
}

func mapsClone(values map[string]float64) map[string]float64 {
	cloned := make(map[string]float64, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
