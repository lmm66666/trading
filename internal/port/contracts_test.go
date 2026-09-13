package port_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

var (
	_ port.MarketData          = (*fakeMarketData)(nil)
	_ port.MarketDataWriter    = (*fakeMarketDataWriter)(nil)
	_ port.MarketSource        = (*fakeMarketSource)(nil)
	_ port.RunStore            = (*fakeRunStore)(nil)
	_ port.JobQueue            = (*fakeJobQueue)(nil)
	_ port.SignalSnapshotStore = (*fakeSnapshotStore)(nil)
	_ port.EventBus            = (*fakeEventBus)(nil)
	_ port.Telemetry           = (*fakeTelemetry)(nil)
)

func TestBatchRequestValidateRejectsInvalidBoundsAndDoesNotMutateAuxiliary(t *testing.T) {
	request := validBatchRequest()
	before := append([]market.Timeframe(nil), request.Auxiliary...)
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !reflect.DeepEqual(request.Auxiliary, before) {
		t.Fatalf("Validate() mutated Auxiliary: got %v, want %v", request.Auxiliary, before)
	}

	tests := []struct {
		name   string
		mutate func(*port.BatchRequest)
	}{
		{"zero version", func(request *port.BatchRequest) { request.Version = 0 }},
		{"duplicate auxiliary", func(request *port.BatchRequest) { request.Auxiliary = []market.Timeframe{market.Week, market.Week} }},
		{"primary auxiliary overlap", func(request *port.BatchRequest) { request.Auxiliary = []market.Timeframe{market.Day} }},
		{"negative lookback", func(request *port.BatchRequest) { request.LookbackBars = -1 }},
		{"too much lookback", func(request *port.BatchRequest) { request.LookbackBars = port.MaxLookbackBars + 1 }},
		{"non UTC range", func(request *port.BatchRequest) { request.From = request.From.In(time.FixedZone("CST", 8*60*60)) }},
		{"reversed range", func(request *port.BatchRequest) { request.From, request.To = request.To, request.From }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validBatchRequest()
			test.mutate(&request)
			if err := request.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
				t.Fatalf("Validate() error = %v, want ErrInvalidPortValue", err)
			}
		})
	}
}

func TestPortRequestLimitsAreEnforced(t *testing.T) {
	validScope := port.InstrumentScope{Exchanges: []market.Exchange{market.SSE, market.SZSE}, ActiveOnly: true, Limit: port.MaxScanInstruments}
	if err := validScope.Validate(); err != nil {
		t.Fatalf("InstrumentScope.Validate() error = %v", err)
	}
	scope := port.InstrumentScope{Exchanges: []market.Exchange{market.SSE}, ActiveOnly: true, Limit: port.MaxScanInstruments + 1}
	if err := scope.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("InstrumentScope.Validate() error = %v", err)
	}
	for _, scope := range []port.InstrumentScope{
		{Exchanges: []market.Exchange{"NYSE"}, Limit: 1},
		{Exchanges: []market.Exchange{market.SSE, market.SSE}, Limit: 1},
	} {
		if err := scope.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
			t.Fatalf("InstrumentScope.Validate() error = %v", err)
		}
	}

	for _, request := range []port.PageRequest{{Limit: 0}, {Limit: port.MaxPageSize + 1}, {AfterSequence: -1, Limit: 1}} {
		if err := request.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
			t.Fatalf("PageRequest.Validate() error = %v", err)
		}
	}
}

func TestPortPayloadValidationCoversStableDTOs(t *testing.T) {
	validBundle := port.Bundle{Quality: port.DataComplete, Auxiliary: map[market.Timeframe]market.Dataset{market.Week: {}}}
	if err := validBundle.Validate(); err != nil {
		t.Fatalf("Bundle.Validate() error = %v", err)
	}
	if err := (port.Bundle{Quality: "bad"}).Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("Bundle.Validate() error = %v", err)
	}
	if err := (port.Bundle{Quality: port.DataComplete, Auxiliary: map[market.Timeframe]market.Dataset{market.UnknownTimeframe: {}}}).Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("Bundle.Validate() error = %v", err)
	}

	batch := validWriteBatch()
	if err := batch.Validate(); err != nil {
		t.Fatalf("MarketWriteBatch.Validate() error = %v", err)
	}
	for _, mutate := range []func(*port.MarketWriteBatch){
		func(batch *port.MarketWriteBatch) { batch.Source = "" },
		func(batch *port.MarketWriteBatch) { batch.Digest = "" },
		func(batch *port.MarketWriteBatch) { batch.Instrument = market.InstrumentID{} },
		func(batch *port.MarketWriteBatch) { batch.Bars = nil; batch.Factors = nil; batch.Actions = nil },
		func(batch *port.MarketWriteBatch) {
			batch.Bars = map[market.Timeframe][]market.Bar{market.UnknownTimeframe: nil}
		},
		func(batch *port.MarketWriteBatch) {
			batch.Bars[market.Day][0].Instrument = market.InstrumentID{Exchange: market.SSE, Code: "600001"}
		},
		func(batch *port.MarketWriteBatch) { batch.Factors[0].Numerator = 0 },
		func(batch *port.MarketWriteBatch) { batch.Actions[0].ID = "" },
	} {
		batch := validWriteBatch()
		mutate(&batch)
		if err := batch.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
			t.Fatalf("MarketWriteBatch.Validate() error = %v", err)
		}
	}

	run := validRun()
	if err := run.Validate(); err != nil {
		t.Fatalf("Run.Validate() error = %v", err)
	}
	for _, mutate := range []func(*port.Run){
		func(run *port.Run) { run.ID = "" },
		func(run *port.Run) { run.Kind = "unknown" },
		func(run *port.Run) { run.Status = "unknown" },
		func(run *port.Run) { run.DataVersion = 0 },
		func(run *port.Run) { run.RequestJSON = []byte("not json") },
		func(run *port.Run) { run.LeaseOwner = "owner" },
		func(run *port.Run) {
			value := time.Now().In(time.FixedZone("CST", 8*60*60))
			run.CancelRequestedAt = &value
		},
	} {
		run := validRun()
		mutate(&run)
		if err := run.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
			t.Fatalf("Run.Validate() error = %v", err)
		}
	}
	if err := (port.Failure{Code: "NETWORK", Message: "retry"}).Validate(); err != nil {
		t.Fatalf("Failure.Validate() error = %v", err)
	}
	if err := (port.Failure{}).Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("Failure.Validate() error = %v", err)
	}
}

func TestSnapshotEventAndTelemetryValidationCoversInvalidBranches(t *testing.T) {
	row := validSnapshot().Rows[0]
	for _, mutate := range []func(*port.SnapshotRow){
		func(row *port.SnapshotRow) { row.Instrument = market.InstrumentID{} },
		func(row *port.SnapshotRow) { row.SignalTime = time.Time{} },
		func(row *port.SnapshotRow) { row.Reason = "" },
		func(row *port.SnapshotRow) { row.Values = map[string]float64{"": 1} },
	} {
		candidate := row
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
			t.Fatalf("SnapshotRow.Validate() error = %v", err)
		}
	}

	snapshot := validSnapshot()
	snapshot.Rows = append(snapshot.Rows, snapshot.Rows[0])
	if err := snapshot.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("SignalSnapshot.Validate() error = %v", err)
	}
	snapshot = validSnapshot()
	snapshot.Failures[snapshot.Rows[0].Instrument] = port.Failure{Code: "bad", Message: "bad"}
	if err := snapshot.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("SignalSnapshot.Validate() error = %v", err)
	}
	snapshot = validSnapshot()
	snapshot.Failures[market.InstrumentID{Exchange: market.SSE, Code: "600001"}] = port.Failure{}
	if err := snapshot.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("SignalSnapshot.Validate() error = %v", err)
	}

	event := port.Event{ID: "event-1", Kind: "run.completed", AggregateID: "run-1", OccurredAt: time.Now().UTC()}
	for _, mutate := range []func(*port.Event){
		func(event *port.Event) { event.ID = "" },
		func(event *port.Event) { event.OccurredAt = time.Time{} },
	} {
		candidate := event
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
			t.Fatalf("Event.Validate() error = %v", err)
		}
	}

	observation := port.StageObservation{RunID: "run-1", Stage: "market_load", StrategyID: "daily_b1_buy", StrategyVersion: "1", DataVersion: 1, Duration: time.Second, Rows: 1}
	if err := observation.Validate(); err != nil {
		t.Fatalf("StageObservation.Validate() error = %v", err)
	}
	observation.Rows = -1
	if err := observation.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("StageObservation.Validate() error = %v", err)
	}
}

func TestStableEnumsAndSnapshotNewestSemantics(t *testing.T) {
	for _, value := range []port.RunKind{port.RunBacktest, port.RunScan} {
		if err := value.Validate(); err != nil {
			t.Fatalf("RunKind %q Validate() error = %v", value, err)
		}
	}
	for _, value := range []port.RunStatus{port.RunPending, port.RunRunning, port.RunSucceeded, port.RunPartialSucceeded, port.RunFailed, port.RunCancelled} {
		if err := value.Validate(); err != nil {
			t.Fatalf("RunStatus %q Validate() error = %v", value, err)
		}
	}
	for _, value := range []port.DataQuality{port.DataComplete, port.DataIncomplete} {
		if err := value.Validate(); err != nil {
			t.Fatalf("DataQuality %q Validate() error = %v", value, err)
		}
	}

	key := port.SnapshotKey{StrategyID: "daily_b1_buy", StrategyVersion: "1", ParametersHash: "abc"}
	if err := key.Validate(); err != nil {
		t.Fatalf("zero AsOf must mean newest snapshot: %v", err)
	}
	key.AsOf = time.Now().In(time.FixedZone("CST", 8*60*60))
	if err := key.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("SnapshotKey.Validate() error = %v", err)
	}
}

func TestSnapshotAndEventValidationRejectUnsafePayloadsAndRemainJSONSerializable(t *testing.T) {
	snapshot := validSnapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("SignalSnapshot.Validate() error = %v", err)
	}
	if _, err := json.Marshal(snapshot); err != nil {
		t.Fatalf("json.Marshal(SignalSnapshot) error = %v", err)
	}

	snapshot.Rows[0].Values["score"] = math.NaN()
	if err := snapshot.Validate(); !errors.Is(err, port.ErrInvalidPortValue) {
		t.Fatalf("SignalSnapshot.Validate() error = %v", err)
	}

	event := port.Event{ID: "event-1", Kind: "run.completed", AggregateID: "run-1", Payload: []byte(`{"ok":true}`), OccurredAt: time.Now().UTC()}
	if err := event.Validate(); err != nil {
		t.Fatalf("Event.Validate() error = %v", err)
	}
}

func validBatchRequest() port.BatchRequest {
	return port.BatchRequest{
		PrimaryTimeframe: market.Day,
		Auxiliary:        []market.Timeframe{market.Week},
		From:             time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		To:               time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
		Version:          1,
		LookbackBars:     100,
	}
}

func validSnapshot() port.SignalSnapshot {
	return port.SignalSnapshot{
		ID:          "snapshot-1",
		RunID:       "run-1",
		Key:         port.SnapshotKey{StrategyID: "daily_b1_buy", StrategyVersion: "1", ParametersHash: "abc"},
		DataVersion: 1,
		Rows: []port.SnapshotRow{{
			Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600000"},
			SignalTime: time.Now().UTC(),
			Reason:     "entry",
			Values:     map[string]float64{"score": 1},
		}},
		Failures: map[market.InstrumentID]port.Failure{},
	}
}

func validWriteBatch() port.MarketWriteBatch {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	return port.MarketWriteBatch{
		Source:     "fixture",
		Instrument: id,
		Digest:     "digest",
		Bars: map[market.Timeframe][]market.Bar{market.Day: {{
			Instrument: id,
			Timeframe:  market.Day,
		}}},
		Factors: []market.AdjustmentFactor{{EffectiveTime: time.Now().UTC(), Numerator: 1, Denominator: 1}},
		Actions: []market.CorporateAction{{ID: "action-1", Instrument: id, ExDate: time.Now().UTC()}},
	}
}

func validRun() port.Run {
	return port.Run{
		ID:              "run-1",
		IdempotencyKey:  "key-1",
		InputHash:       "hash-1",
		Kind:            port.RunBacktest,
		Status:          port.RunPending,
		StrategyID:      "daily_b1_buy",
		StrategyVersion: "1",
		EngineVersion:   "1",
		DataVersion:     1,
		RequestJSON:     []byte(`{"instrument":"SSE:600000"}`),
	}
}

type fakeMarketData struct{}

func (*fakeMarketData) LatestCompleteVersion(context.Context) (market.DataVersion, error) {
	return 0, nil
}
func (*fakeMarketData) Dataset(context.Context, market.InstrumentID, market.Timeframe, time.Time, time.Time, market.DataVersion) (market.Dataset, []market.AdjustmentFactor, []market.CorporateAction, error) {
	return market.Dataset{}, nil, nil, nil
}
func (*fakeMarketData) BatchDatasets(context.Context, []market.InstrumentID, port.BatchRequest) (map[market.InstrumentID]port.Bundle, map[market.InstrumentID]error) {
	return nil, nil
}
func (*fakeMarketData) Instruments(context.Context, port.InstrumentScope) ([]market.InstrumentID, error) {
	return nil, nil
}
func (*fakeMarketData) DirtyInstruments(context.Context, market.DataVersion, market.DataVersion) ([]market.InstrumentID, error) {
	return nil, nil
}

type fakeMarketDataWriter struct{}

func (*fakeMarketDataWriter) Publish(context.Context, port.MarketWriteBatch) (market.DataVersion, error) {
	return 0, nil
}

type fakeMarketSource struct{}

func (*fakeMarketSource) FetchBars(context.Context, market.InstrumentID, market.Timeframe, time.Time, time.Time) ([]market.Bar, []market.AdjustmentFactor, error) {
	return nil, nil, nil
}
func (*fakeMarketSource) FetchCorporateActions(context.Context, market.InstrumentID) ([]market.CorporateAction, error) {
	return nil, nil
}

type fakeRunStore struct{}

func (*fakeRunStore) CompleteBacktest(context.Context, string, string, backtest.Result) error {
	return nil
}
func (*fakeRunStore) CompleteScan(context.Context, string, string, port.SignalSnapshot) error {
	return nil
}
func (*fakeRunStore) Fail(context.Context, string, string, port.Failure) error { return nil }
func (*fakeRunStore) Get(context.Context, string) (port.Run, error)            { return port.Run{}, nil }
func (*fakeRunStore) BacktestResult(context.Context, string) (backtest.Summary, error) {
	return backtest.Summary{}, nil
}
func (*fakeRunStore) Orders(context.Context, string, port.PageRequest) (port.Page[backtest.Order], error) {
	return port.Page[backtest.Order]{}, nil
}
func (*fakeRunStore) Trades(context.Context, string, port.PageRequest) (port.Page[backtest.Fill], error) {
	return port.Page[backtest.Fill]{}, nil
}
func (*fakeRunStore) Equity(context.Context, string, port.PageRequest) (port.Page[backtest.EquityPoint], error) {
	return port.Page[backtest.EquityPoint]{}, nil
}

type fakeJobQueue struct{}

func (*fakeJobQueue) Enqueue(context.Context, port.Run) (port.Run, error) { return port.Run{}, nil }
func (*fakeJobQueue) Claim(context.Context, string, time.Duration) (port.Run, error) {
	return port.Run{}, nil
}
func (*fakeJobQueue) Renew(context.Context, string, string, time.Duration) error { return nil }
func (*fakeJobQueue) Retry(context.Context, string, string, time.Time, port.Failure) error {
	return nil
}
func (*fakeJobQueue) RequestCancel(context.Context, string) error { return nil }

type fakeSnapshotStore struct{}

func (*fakeSnapshotStore) Latest(context.Context, port.SnapshotKey, port.PageRequest) (port.SignalSnapshot, error) {
	return port.SignalSnapshot{}, nil
}

type fakeEventBus struct{}

func (*fakeEventBus) Publish(context.Context, port.Event) error { return nil }

type fakeTelemetry struct{}

func (*fakeTelemetry) ObserveStage(context.Context, port.StageObservation) {}
func (*fakeTelemetry) CountRetry(context.Context, string, string, int)     {}
