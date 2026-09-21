package application

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"trading/internal/market"
	"trading/internal/port"
)

type FuturesScheduler struct {
	progress  *RefreshProgress
	refresher MarketRefresher
	ids       []market.InstrumentID
	logger    *slog.Logger
	running   atomic.Bool
	started   atomic.Bool
	mu        sync.RWMutex
	last      RefreshSummary
	async     sync.WaitGroup
}

func DefaultSinaFuturesInstruments() []market.InstrumentID {
	return []market.InstrumentID{
		{Exchange: market.SHFE, Code: "AU.MAIN"},
		{Exchange: market.SHFE, Code: "AG.MAIN"},
		{Exchange: market.SHFE, Code: "FU.MAIN"},
		{Exchange: market.INE, Code: "SC.MAIN"},
		{Exchange: market.INE, Code: "LU.MAIN"},
		{Exchange: market.DCE, Code: "J.MAIN"},
		{Exchange: market.DCE, Code: "JM.MAIN"},
		{Exchange: market.CZCE, Code: "ZC.MAIN"},
	}
}

func NewFuturesScheduler(refresher MarketRefresher, ids []market.InstrumentID, logger *slog.Logger) (*FuturesScheduler, error) {
	if refresher == nil || len(ids) == 0 || logger == nil {
		return nil, invalidRequest("futures scheduler dependencies are required")
	}
	owned := append([]market.InstrumentID(nil), ids...)
	seen := make(map[market.InstrumentID]struct{}, len(owned))
	for _, id := range owned {
		if err := id.Validate(); err != nil || id.Kind() != market.FuturesContinuous {
			return nil, invalidRequest("invalid futures scheduler instrument")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, invalidRequest("duplicate futures scheduler instrument")
		}
		seen[id] = struct{}{}
	}
	return &FuturesScheduler{refresher: refresher, ids: owned, logger: logger}, nil
}

func (scheduler *FuturesScheduler) SetProgress(p *RefreshProgress) { scheduler.progress = p }

func (scheduler *FuturesScheduler) RunOnce(ctx context.Context) RefreshSummary {
	if !scheduler.running.CompareAndSwap(false, true) {
		return RefreshSummary{Err: ErrRefreshAlreadyRunning}
	}
	defer scheduler.running.Store(false)
	observation, _ := scheduler.progress.Begin(ctx, "FUTURES", "SCHEDULED")
	return scheduler.runOnce(ctx, observation)
}

func (scheduler *FuturesScheduler) TriggerTracked(ctx context.Context) (port.RefreshReceipt, error) {
	if err := ctx.Err(); err != nil {
		return port.RefreshReceipt{}, err
	}
	if !scheduler.running.CompareAndSwap(false, true) {
		return port.RefreshReceipt{}, ErrRefreshAlreadyRunning
	}
	observation, receipt := scheduler.progress.Begin(ctx, "FUTURES", "MANUAL")
	scheduler.async.Add(1)
	go func() {
		defer scheduler.async.Done()
		defer scheduler.running.Store(false)
		scheduler.runOnce(ctx, observation)
	}()
	return receipt, nil
}

func (scheduler *FuturesScheduler) Wait() { scheduler.async.Wait() }

func (scheduler *FuturesScheduler) runOnce(ctx context.Context, observation *RefreshObservation) RefreshSummary {
	startedAt := time.Now()
	summary := RefreshSummary{Total: len(scheduler.ids), Results: make(map[market.InstrumentID]RefreshResult, len(scheduler.ids)), Failures: make(map[market.InstrumentID]error)}

	observation.Prepared(len(scheduler.ids))
	defer func() { observation.End(summary.Err) }()
	defer func() {
		scheduler.mu.Lock()
		scheduler.last = cloneRefreshSummary(summary)
		scheduler.mu.Unlock()
		attributes := []any{
			"component", "futures_scheduler",
			"operation", "refresh",
			"total", summary.Total,
			"succeeded", len(summary.Results),
			"failed", len(summary.Failures),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		}
		if summary.Err != nil || len(summary.Failures) > 0 {
			scheduler.logger.WarnContext(ctx, "futures refresh completed with failures", attributes...)
			return
		}
		scheduler.logger.InfoContext(ctx, "futures refresh completed", attributes...)
	}()
	for _, id := range scheduler.ids {
		if err := ctx.Err(); err != nil {
			summary.Err = err
			break
		}
		result, err := scheduler.refresher.Refresh(ctx, id)
		observation.Completed(id, err)
		if err != nil {
			summary.Failures[id] = err
			if ctx.Err() != nil {
				summary.Err = ctx.Err()
				break
			}
			continue
		}
		summary.Results[id] = result
	}
	if summary.Err != nil {
		for _, id := range scheduler.ids {
			if _, ok := summary.Results[id]; !ok && summary.Failures[id] == nil {
				summary.Failures[id] = summary.Err
			}
		}
	}
	return summary
}

func (scheduler *FuturesScheduler) Start(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return invalidRequest("invalid futures scheduler interval")
	}
	if !scheduler.started.CompareAndSwap(false, true) {
		return ErrRefreshAlreadyRunning
	}
	defer scheduler.started.Store(false)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		summary := scheduler.RunOnce(ctx)
		if summary.Err != nil && !errors.Is(summary.Err, ErrRefreshAlreadyRunning) {
			return summary.Err
		}
	}
}

func (scheduler *FuturesScheduler) LastSummary() RefreshSummary {
	scheduler.mu.RLock()
	defer scheduler.mu.RUnlock()
	return cloneRefreshSummary(scheduler.last)
}
