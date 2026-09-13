package application

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"trading/internal/backtest"
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

func (s *BacktestService) Execute(ctx context.Context, run port.Run) error {
	if err := validateClaim(run, port.RunBacktest, s.config.EngineVersion); err != nil {
		return err
	}
	if err := checkRun(ctx, s.store, run); err != nil {
		return err
	}
	var req BacktestRequest
	if err := json.Unmarshal(run.RequestJSON, &req); err != nil {
		return invalidRequest("invalid saved backtest")
	}
	if err := req.Validate(); err != nil {
		return err
	}
	definition, _, err := resolveComputeStrategy(s.registry, req.StrategyID, req.StrategyVersion, req.Parameters)
	if err != nil {
		return err
	}
	hash, err := canonicalInputHash(req, definition, run.DataVersion, run.EngineVersion)
	if err != nil {
		return err
	}
	if hash != run.InputHash || req.StrategyID != run.StrategyID || req.StrategyVersion != run.StrategyVersion {
		return invalidRequest("saved backtest inputs changed")
	}
	start := s.config.Clock()
	bundles, failures := s.market.BatchDatasets(ctx, []market.InstrumentID{req.Instrument}, port.BatchRequest{PrimaryTimeframe: definition.PrimaryTimeframe, Auxiliary: definition.Auxiliary, From: req.Start, To: req.End, Version: run.DataVersion, LookbackBars: definition.WarmupBars})
	s.config.observe(ctx, run, "market_load", start, 0, 0, 0, 0)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := failures[req.Instrument]; err != nil {
		return err
	}
	bundle, ok := bundles[req.Instrument]
	if !ok {
		return ErrIncompleteMarketData
	}
	start = s.config.Clock()
	err = validateComputeBundle(bundle, req.Instrument, definition, run.DataVersion)
	s.config.observe(ctx, run, "dataset_validation", start, int64(bundle.Primary.Len()), 0, 0, 0)
	if err != nil {
		return err
	}
	start = s.config.Clock()
	timeline, err := buildComputeTimeline(bundle, definition)
	s.config.observe(ctx, run, "feature_graph", start, int64(bundle.Primary.Len()), 0, 0, 0)
	if err != nil {
		return err
	}
	timeline, err = backtestWindow(timeline, req.Start, req.End)
	if err != nil {
		return err
	}
	actions := make([]market.CorporateAction, 0, len(bundle.Actions))
	for _, action := range bundle.Actions {
		if !action.ExDate.Before(timeline.Primary.Bar(0).OpenTime) && !action.ExDate.After(timeline.Primary.Bar(timeline.Len()-1).CloseTime) {
			actions = append(actions, action)
		}
	}
	instance, err := s.registry.Resolve(req.StrategyID, req.StrategyVersion, req.Parameters)
	if err != nil {
		return err
	}
	if err = checkRun(ctx, s.store, run); err != nil {
		return err
	}
	start = s.config.Clock()
	result, err := s.engine.Run(ctx, backtest.Input{Strategy: instance, Timeline: timeline, Actions: actions, Config: req.Config})
	s.config.observe(ctx, run, "engine_execution", start, int64(timeline.Len()), 0, 0, 0)
	if err != nil {
		return err
	}
	if err = checkRun(ctx, s.store, run); err != nil {
		return err
	}
	start = s.config.Clock()
	err = s.store.CompleteBacktest(ctx, run.ID, run.LeaseToken, result)
	s.config.observe(ctx, run, "persistence", start, int64(len(result.Equity)), 0, 0, 0)
	return err
}

// Warmup bars feed indicators, but account state begins at the requested
// window. Slicing preserves already computed values and auxiliary alignment.
func backtestWindow(timeline strategy.Timeline, from, to time.Time) (strategy.Timeline, error) {
	start := sort.Search(timeline.Len(), func(i int) bool { return !timeline.Primary.Bar(i).CloseTime.Before(from) })
	end := sort.Search(timeline.Len(), func(i int) bool { return timeline.Primary.Bar(i).CloseTime.After(to) })
	if start >= end {
		return strategy.Timeline{}, ErrIncompleteMarketData
	}
	primary, err := market.NewDataset(timeline.Primary.Instrument(), timeline.Primary.Timeframe(), timeline.Primary.Version(), timeline.Primary.Bars()[start:end])
	if err != nil {
		return strategy.Timeline{}, err
	}
	features := make(indicator.Set, len(timeline.Features))
	for key, value := range timeline.Features {
		features[key] = value.Slice(start, end)
	}
	auxiliary := make(map[market.Timeframe]strategy.AlignedFeatures, len(timeline.Auxiliary))
	for tf, aligned := range timeline.Auxiliary {
		aligned.PrimaryToAuxiliary = aligned.PrimaryToAuxiliary[start:end]
		auxiliary[tf] = aligned
	}
	return strategy.NewTimeline(primary, features, auxiliary)
}
func validateClaim(run port.Run, kind port.RunKind, engineVersion string) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if run.Kind != kind || run.Status != port.RunRunning || run.Attempts < 1 || run.EngineVersion != engineVersion {
		return invalidRequest("invalid claimed run or engine version")
	}
	return nil
}
func checkRun(ctx context.Context, store port.RunStore, run port.Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := store.Get(ctx, run.ID)
	if err != nil {
		return err
	}
	if current.CancelRequestedAt != nil || current.Status == port.RunCancelled {
		return context.Canceled
	}
	if current.Status != port.RunRunning || current.LeaseToken != run.LeaseToken {
		return port.ErrLeaseLost
	}
	return nil
}
func validateComputeBundle(bundle port.Bundle, id market.InstrumentID, definition strategy.Definition, version market.DataVersion) error {
	if bundle.Validate() != nil || bundle.Quality != port.DataComplete {
		return ErrIncompleteMarketData
	}
	datasets := []market.Dataset{bundle.Primary}
	if bundle.Primary.Timeframe() != definition.PrimaryTimeframe || bundle.Primary.Len() == 0 {
		return ErrIncompleteMarketData
	}
	for _, tf := range definition.Auxiliary {
		d, ok := bundle.Auxiliary[tf]
		if !ok || d.Timeframe() != tf {
			return ErrIncompleteMarketData
		}
		datasets = append(datasets, d)
	}
	for _, d := range datasets {
		if d.Instrument() != id || d.Version() != version {
			return ErrIncompleteMarketData
		}
		for _, b := range d.Bars() {
			if b.OpenTime.IsZero() || b.CloseTime.IsZero() || b.OpenTime.After(b.CloseTime) || b.OpenTime.Location() != time.UTC || b.CloseTime.Location() != time.UTC {
				return ErrIncompleteMarketData
			}
		}
	}
	for _, f := range bundle.Factors {
		if f.Version != version || f.Numerator <= 0 || f.Denominator <= 0 {
			return ErrIncompleteMarketData
		}
	}
	if _, err := normalizeFactors(bundle.Factors); err != nil {
		return ErrIncompleteMarketData
	}
	for _, a := range bundle.Actions {
		if a.Version != version || a.Instrument != id {
			return ErrIncompleteMarketData
		}
	}
	return nil
}
func buildComputeTimeline(bundle port.Bundle, definition strategy.Definition) (strategy.Timeline, error) {
	refs := map[market.Timeframe][]indicator.Ref{}
	for _, ref := range definition.Features {
		refs[ref.Timeframe] = append(refs[ref.Timeframe], ref)
	}
	features, err := indicator.Build(bundle.Primary, bundle.Factors, refs[definition.PrimaryTimeframe])
	if err != nil {
		return strategy.Timeline{}, err
	}
	auxiliary := make(map[market.Timeframe]strategy.AlignedFeatures, len(definition.Auxiliary))
	for _, tf := range definition.Auxiliary {
		dataset := bundle.Auxiliary[tf]
		set, err := indicator.Build(dataset, bundle.Factors, refs[tf])
		if err != nil {
			return strategy.Timeline{}, err
		}
		mapping := make([]int, bundle.Primary.Len())
		for i := range mapping {
			at := bundle.Primary.Bar(i).CloseTime
			mapping[i] = sort.Search(dataset.Len(), func(j int) bool { return dataset.Bar(j).CloseTime.After(at) }) - 1
		}
		auxiliary[tf] = strategy.AlignedFeatures{Dataset: dataset, Features: set, PrimaryToAuxiliary: mapping}
	}
	return strategy.NewTimeline(bundle.Primary, features, auxiliary)
}
