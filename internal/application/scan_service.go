package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

type ScanRequest struct {
	StrategyID      string               `json:"strategy_id"`
	StrategyVersion string               `json:"strategy_version"`
	IdempotencyKey  string               `json:"idempotency_key"`
	Parameters      map[string]float64   `json:"parameters,omitempty"`
	Scope           port.InstrumentScope `json:"scope"`
	From            time.Time            `json:"from"`
	AsOf            time.Time            `json:"as_of"`
}

func (r ScanRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := validateUTCDate(r.From, "from"); err != nil {
		return err
	}
	if err := validateUTCDate(r.AsOf, "as of"); err != nil {
		return err
	}
	if r.AsOf.Before(r.From) || r.AsOf.After(r.From.AddDate(MaxBacktestRangeYears, 0, 0)) {
		return ErrDateRangeTooLarge
	}
	for _, f := range []struct {
		v, n string
		max  int
	}{{r.StrategyID, "strategy ID", port.MaxStrategyIDBytes}, {r.StrategyVersion, "strategy version", port.MaxStrategyVersionBytes}, {r.IdempotencyKey, "idempotency key", port.MaxIdempotencyKeyBytes}} {
		if err := port.ValidateIdentity(f.v, f.n, f.max, false); err != nil {
			return err
		}
	}
	return nil
}

type ScanConfig struct {
	ComputeConfig
	Workers int
}
type scanInput struct {
	ScanRequest
	Instruments        []market.InstrumentID `json:"instruments"`
	ParametersHash     string                `json:"parameters_hash"`
	PreviousSnapshotID string                `json:"previous_snapshot_id,omitempty"`
}
type ScanService struct {
	registry  *strategy.Registry
	market    port.MarketData
	queue     port.JobQueue
	store     port.RunStore
	snapshots port.SignalSnapshotStore
	config    ScanConfig
}

func NewScanService(registry *strategy.Registry, data port.MarketData, queue port.JobQueue, store port.RunStore, snapshots port.SignalSnapshotStore, config ScanConfig) (*ScanService, error) {
	if nilComputeDependency(registry) || nilComputeDependency(data) || nilComputeDependency(queue) || nilComputeDependency(store) || nilComputeDependency(snapshots) {
		return nil, invalidRequest("scan dependencies are required")
	}
	if err := config.defaults(); err != nil {
		return nil, err
	}
	if config.Workers < 1 || config.Workers > 64 {
		return nil, invalidRequest("scan workers must be between 1 and 64")
	}
	return &ScanService{registry, data, queue, store, snapshots, config}, nil
}
func (s *ScanService) Create(ctx context.Context, request ScanRequest) (port.Run, error) {
	if err := ctx.Err(); err != nil {
		return port.Run{}, err
	}
	request.From = request.From.UTC().Truncate(time.Microsecond)
	request.AsOf = request.AsOf.UTC().Truncate(time.Microsecond)
	if err := request.Validate(); err != nil {
		return port.Run{}, err
	}
	definition, params, err := resolveComputeStrategy(s.registry, request.StrategyID, request.StrategyVersion, request.Parameters)
	if err != nil {
		return port.Run{}, err
	}
	request.Parameters = params
	request.Scope.Exchanges = append([]market.Exchange(nil), request.Scope.Exchanges...)
	sort.Slice(request.Scope.Exchanges, func(i, j int) bool { return request.Scope.Exchanges[i] < request.Scope.Exchanges[j] })
	if previous, found, err := previousSubmission(ctx, s.queue, port.RunScan, request.IdempotencyKey); err != nil {
		return port.Run{}, err
	} else if found {
		return reuseScanSubmission(s.registry, request, previous)
	}
	version, err := s.market.LatestCompleteVersion(ctx)
	if err != nil {
		return port.Run{}, err
	}
	if version == 0 {
		return port.Run{}, port.ErrMarketDataNotFound
	}
	ids, err := s.market.Instruments(ctx, request.Scope)
	if err != nil {
		return port.Run{}, err
	}
	ids, err = canonicalScanIDs(ids, request.Scope.Limit)
	if err != nil {
		return port.Run{}, err
	}
	input := scanInput{ScanRequest: request, Instruments: ids}
	input.ParametersHash, err = scanParametersHash(input, definition, s.config.EngineVersion)
	if err != nil {
		return port.Run{}, err
	}
	previous, err := s.snapshots.Latest(ctx, port.SnapshotKey{StrategyID: request.StrategyID, StrategyVersion: request.StrategyVersion, ParametersHash: input.ParametersHash, AsOf: request.AsOf}, port.PageRequest{Limit: 1})
	if err != nil && !errors.Is(err, port.ErrSnapshotNotReady) {
		return port.Run{}, err
	}
	if err == nil && previous.DataVersion <= version {
		input.PreviousSnapshotID = previous.ID
	}
	hash, err := scanInputHash(input, definition, version, s.config.EngineVersion)
	if err != nil {
		return port.Run{}, err
	}
	run, err := enqueueCompute(ctx, s.queue, port.RunScan, request.StrategyID, request.StrategyVersion, request.IdempotencyKey, hash, version, s.config.EngineVersion, input)
	if err == nil {
		return reuseScanSubmission(s.registry, request, run)
	}
	winner, found, err := collidedSubmission(ctx, s.queue, port.RunScan, request.IdempotencyKey, err)
	if !found {
		return port.Run{}, err
	}
	return reuseScanSubmission(s.registry, request, winner)
}
func (s *ScanService) Cancel(ctx context.Context, id string) error {
	return s.queue.RequestCancel(ctx, id)
}
func (s *ScanService) Latest(ctx context.Context, key port.SnapshotKey, page port.PageRequest) (port.SignalSnapshot, error) {
	if err := key.Validate(); err != nil {
		return port.SignalSnapshot{}, err
	}
	if err := page.Validate(); err != nil {
		return port.SignalSnapshot{}, err
	}
	if page.AfterSequence > 0 && key.SnapshotID == "" {
		return port.SignalSnapshot{}, invalidRequest("snapshot ID required for continuation")
	}
	return s.snapshots.Latest(ctx, key, page)
}
func canonicalScanIDs(ids []market.InstrumentID, limit int) ([]market.InstrumentID, error) {
	if len(ids) > limit || len(ids) > port.MaxScanInstruments {
		return nil, invalidRequest("too many scan instruments")
	}
	seen := map[market.InstrumentID]bool{}
	out := make([]market.InstrumentID, 0, len(ids))
	for _, id := range ids {
		if err := id.Validate(); err != nil {
			return nil, err
		}
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}
func scanParametersHash(input scanInput, definition strategy.Definition, engine string) (string, error) {
	input.IdempotencyKey = ""
	input.AsOf = time.Time{}
	input.ParametersHash = ""
	input.PreviousSnapshotID = ""
	return computeHash(struct {
		Input      scanInput
		Definition strategy.Definition
		Engine     string
	}{input, definition, engine})
}
func scanInputHash(input scanInput, definition strategy.Definition, version market.DataVersion, engine string) (string, error) {
	input.IdempotencyKey = ""
	return computeHash(struct {
		Input      scanInput
		Definition strategy.Definition
		Version    market.DataVersion
		Engine     string
	}{input, definition, version, engine})
}
func (s *ScanService) Execute(ctx context.Context, run port.Run) error {
	if err := validateClaim(run, port.RunScan, s.config.EngineVersion); err != nil {
		return err
	}
	if err := checkRun(ctx, s.store, run); err != nil {
		return err
	}
	var input scanInput
	if err := json.Unmarshal(run.RequestJSON, &input); err != nil {
		return invalidRequest("invalid saved scan")
	}
	if err := input.Validate(); err != nil {
		return err
	}
	definition, _, err := resolveComputeStrategy(s.registry, input.StrategyID, input.StrategyVersion, input.Parameters)
	if err != nil {
		return err
	}
	hash, err := scanInputHash(input, definition, run.DataVersion, run.EngineVersion)
	if err != nil {
		return err
	}
	paramsHash, err := scanParametersHash(input, definition, run.EngineVersion)
	if err != nil {
		return err
	}
	if hash != run.InputHash || paramsHash != input.ParametersHash || input.StrategyID != run.StrategyID || input.StrategyVersion != run.StrategyVersion {
		return invalidRequest("saved scan inputs changed")
	}
	ids, err := canonicalScanIDs(input.Instruments, input.Scope.Limit)
	if err != nil {
		return err
	}
	snapshot := port.SignalSnapshot{ID: run.ID, RunID: run.ID, Key: port.SnapshotKey{SnapshotID: run.ID, StrategyID: run.StrategyID, StrategyVersion: run.StrategyVersion, ParametersHash: input.ParametersHash, AsOf: input.AsOf}, DataVersion: run.DataVersion, Rows: []port.SnapshotRow{}, Failures: map[market.InstrumentID]port.Failure{}}
	ids, err = s.mergeBase(ctx, run, input, ids, &snapshot)
	if err != nil {
		return err
	}
	// One port batch accepts the complete bounded universe (at most 5000 IDs).
	// Its adapter groups every requested timeframe rather than querying per ID.
	if len(ids) > 0 {
		if err = checkRun(ctx, s.store, run); err != nil {
			return err
		}
		start := s.config.Clock()
		bundles, failures := s.market.BatchDatasets(ctx, ids, port.BatchRequest{PrimaryTimeframe: definition.PrimaryTimeframe, Auxiliary: definition.Auxiliary, From: input.From, To: input.AsOf, Version: run.DataVersion, LookbackBars: definition.WarmupBars})
		s.config.observe(ctx, run, "market_load", start, 0, 0, 0, 0)
		if err = checkRun(ctx, s.store, run); err != nil {
			return err
		}
		result := scanBatch(ctx, s.config.Workers, ids, func(ctx context.Context, id market.InstrumentID) (port.SnapshotRow, bool, error) {
			if err := failures[id]; err != nil {
				return port.SnapshotRow{}, false, err
			}
			bundle, ok := bundles[id]
			if !ok {
				return port.SnapshotRow{}, false, ErrIncompleteMarketData
			}
			return s.replay(ctx, run, input, definition, id, bundle)
		})
		if err = ctx.Err(); err != nil {
			return err
		}
		for id, failure := range result.failures {
			snapshot.Failures[id] = failure
		}
		snapshot.Rows = append(snapshot.Rows, result.rows...)
	}
	sort.Slice(snapshot.Rows, func(i, j int) bool {
		return snapshot.Rows[i].Instrument.String() < snapshot.Rows[j].Instrument.String()
	})
	if err = snapshot.Validate(); err != nil {
		return err
	}
	if err = checkRun(ctx, s.store, run); err != nil {
		return err
	}
	start := s.config.Clock()
	err = s.store.CompleteScan(ctx, run.ID, run.LeaseToken, snapshot)
	s.config.observe(ctx, run, "persistence", start, int64(len(snapshot.Rows)), int64(len(snapshot.Rows)), int64(len(snapshot.Failures)), int64(len(input.Instruments)-len(snapshot.Rows)-len(snapshot.Failures)))
	return err
}
