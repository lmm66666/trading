package application

import (
	"context"
	"sync"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

type scanBatchResult struct {
	rows     []port.SnapshotRow
	failures map[market.InstrumentID]port.Failure
}

func scanBatch(ctx context.Context, workers int, ids []market.InstrumentID, fn func(context.Context, market.InstrumentID) (port.SnapshotRow, bool, error)) scanBatchResult {
	result := scanBatchResult{failures: map[market.InstrumentID]port.Failure{}}
	jobs := make(chan market.InstrumentID)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < min(workers, len(ids)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if ctx.Err() != nil {
					continue
				}
				row, signal, err := fn(ctx, id)
				mu.Lock()
				if err != nil {
					result.failures[id] = classifyFailure(err)
				} else if signal {
					result.rows = append(result.rows, row)
				}
				mu.Unlock()
			}
		}()
	}
produce:
	for _, id := range ids {
		select {
		case <-ctx.Done():
			break produce
		case jobs <- id:
		}
	}
	close(jobs)
	wg.Wait()
	return result
}
func (s *ScanService) replay(ctx context.Context, run port.Run, input scanInput, definition strategy.Definition, id market.InstrumentID, bundle port.Bundle) (port.SnapshotRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return port.SnapshotRow{}, false, err
	}
	start := s.config.Clock()
	err := validateComputeBundle(bundle, id, definition, run.DataVersion)
	s.config.observe(ctx, run, "dataset_validation", start, int64(bundle.Primary.Len()), 0, 0, 0)
	if err != nil {
		return port.SnapshotRow{}, false, err
	}
	lastClose := bundle.Primary.Bar(bundle.Primary.Len() - 1).CloseTime
	if lastClose.Before(input.From) || lastClose.After(input.AsOf) {
		return port.SnapshotRow{}, false, ErrIncompleteMarketData
	}
	start = s.config.Clock()
	timeline, err := buildComputeTimeline(bundle, definition)
	s.config.observe(ctx, run, "feature_graph", start, int64(bundle.Primary.Len()), 0, 0, 0)
	if err != nil {
		return port.SnapshotRow{}, false, err
	}
	instance, err := s.registry.Resolve(input.StrategyID, input.StrategyVersion, input.Parameters)
	if err != nil {
		return port.SnapshotRow{}, false, err
	}
	start = s.config.Clock()
	decision, err := strategy.ReplayLatest(&cancelStrategy{Strategy: instance, ctx: ctx}, timeline)
	s.config.observe(ctx, run, "strategy_replay", start, int64(bundle.Primary.Len()), 0, 0, 0)
	if err != nil {
		return port.SnapshotRow{}, false, err
	}
	if decision.Action > strategy.ExitLong {
		return port.SnapshotRow{}, false, invalidRequest("invalid strategy action")
	}
	if decision.Action != strategy.EnterLong {
		return port.SnapshotRow{}, false, nil
	}
	row := port.SnapshotRow{Instrument: id, SignalTime: bundle.Primary.Bar(bundle.Primary.Len() - 1).CloseTime, Reason: decision.Reason, Values: decision.Values}
	if err = row.Validate(); err != nil {
		return port.SnapshotRow{}, false, err
	}
	return row, true, nil
}

type cancelStrategy struct {
	strategy.Strategy
	ctx context.Context
}

func (s *cancelStrategy) OnBar(ctx strategy.Context) (strategy.Decision, error) {
	if err := s.ctx.Err(); err != nil {
		return strategy.Decision{}, err
	}
	return s.Strategy.OnBar(ctx)
}

func (s *ScanService) mergeBase(ctx context.Context, run port.Run, input scanInput, ids []market.InstrumentID, out *port.SignalSnapshot) ([]market.InstrumentID, error) {
	reader, ok := s.market.(port.MarketChangeReader)
	if !ok || input.PreviousSnapshotID == "" {
		return ids, nil
	}
	key := out.Key
	key.SnapshotID = input.PreviousSnapshotID
	page := port.PageRequest{Limit: port.MaxPageSize}
	base, err := s.snapshots.Latest(ctx, key, page)
	if err != nil {
		return nil, err
	}
	if base.ID != input.PreviousSnapshotID || base.DataVersion > run.DataVersion {
		return nil, invalidRequest("invalid previous snapshot")
	}
	changes, err := reader.MarketChanges(ctx, base.DataVersion, run.DataVersion)
	if err != nil {
		return nil, err
	}
	if changes.FactorsOrActionsChanged {
		return ids, nil
	}
	dirty := map[market.InstrumentID]bool{}
	for _, id := range changes.Dirty {
		dirty[id] = true
	}
	for id := range base.Failures {
		dirty[id] = true
	}
	current := map[market.InstrumentID]bool{}
	for _, id := range ids {
		current[id] = true
	}
	for {
		for _, row := range base.Rows {
			if current[row.Instrument] && !dirty[row.Instrument] {
				out.Rows = append(out.Rows, row)
			}
		}
		if len(base.Rows) < port.MaxPageSize {
			break
		}
		page.AfterSequence += int64(len(base.Rows))
		if page.AfterSequence > port.MaxScanInstruments {
			return nil, invalidRequest("oversized previous snapshot")
		}
		if err = checkRun(ctx, s.store, run); err != nil {
			return nil, err
		}
		next, err := s.snapshots.Latest(ctx, key, page)
		if err != nil {
			return nil, err
		}
		if next.ID != base.ID || next.DataVersion != base.DataVersion {
			return nil, invalidRequest("snapshot changed during pagination")
		}
		base = next
	}
	selected := make([]market.InstrumentID, 0, len(dirty))
	for _, id := range ids {
		if dirty[id] {
			selected = append(selected, id)
		}
	}
	return selected, nil
}
