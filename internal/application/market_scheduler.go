package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

const MaxMarketWorkers = 64

type MarketRefresher interface {
	Refresh(context.Context, market.InstrumentID) (RefreshResult, error)
}
type RefreshSummary struct {
	Total    int
	Results  map[market.InstrumentID]RefreshResult
	Failures map[market.InstrumentID]error
	Err      error
}
type MarketScheduler struct {
	progress  *RefreshProgress
	data      port.MarketData
	refresher MarketRefresher
	scope     port.InstrumentScope
	started   atomic.Bool
	mu        sync.RWMutex
	last      RefreshSummary
	async     sync.WaitGroup
}

// 所有定时及手工全市场触发共用进程内 guard。跨进程协调由部署单实例保证。
var marketRefreshRunning atomic.Bool

func NewMarketScheduler(data port.MarketData, refresher MarketRefresher, scope port.InstrumentScope) (*MarketScheduler, error) {
	if data == nil || refresher == nil {
		return nil, invalidRequest("market scheduler dependencies are required")
	}
	if err := scope.Validate(); err != nil {
		return nil, invalidRequest("invalid market scope")
	}
	scope.Exchanges = append([]market.Exchange(nil), scope.Exchanges...)
	return &MarketScheduler{data: data, refresher: refresher, scope: scope}, nil
}
func (s *MarketScheduler) RunOnce(ctx context.Context, workers int) RefreshSummary {
	summary := RefreshSummary{Results: map[market.InstrumentID]RefreshResult{}, Failures: map[market.InstrumentID]error{}}
	if workers < 1 || workers > MaxMarketWorkers {
		summary.Err = invalidRequest("invalid worker count")
		return summary
	}
	if !marketRefreshRunning.CompareAndSwap(false, true) {
		summary.Err = ErrRefreshAlreadyRunning
		return summary
	}
	defer marketRefreshRunning.Store(false)
	h, _ := s.progress.Begin(ctx, "STOCK", "SCHEDULED")
	return s.runOnce(ctx, workers, h)
}

// TriggerNow starts one managed refresh without tying its lifetime to an HTTP
// request. The caller supplies the application root context; Wait joins all
// accepted manual triggers before the database is closed.
func (s *MarketScheduler) SetProgress(p *RefreshProgress) { s.progress = p }
func (s *MarketScheduler) TriggerNow(ctx context.Context, workers int) error {
	_, err := s.TriggerTracked(ctx, workers)
	return err
}
func (s *MarketScheduler) TriggerTracked(ctx context.Context, workers int) (port.RefreshReceipt, error) {
	if workers < 1 || workers > MaxMarketWorkers {
		return port.RefreshReceipt{}, invalidRequest("invalid worker count")
	}
	if err := ctx.Err(); err != nil {
		return port.RefreshReceipt{}, err
	}
	if !marketRefreshRunning.CompareAndSwap(false, true) {
		return port.RefreshReceipt{}, ErrRefreshAlreadyRunning
	}
	h, receipt := s.progress.Begin(ctx, "STOCK", "MANUAL")
	s.async.Add(1)
	go func() { defer s.async.Done(); defer marketRefreshRunning.Store(false); s.runOnce(ctx, workers, h) }()
	return receipt, nil
}

func (s *MarketScheduler) Wait() { s.async.Wait() }

func (s *MarketScheduler) runOnce(ctx context.Context, workers int, observation *RefreshObservation) RefreshSummary {
	summary := RefreshSummary{Results: map[market.InstrumentID]RefreshResult{}, Failures: map[market.InstrumentID]error{}}
	defer func() { observation.End(summary.Err) }()
	defer func() { s.mu.Lock(); s.last = cloneRefreshSummary(summary); s.mu.Unlock() }()
	if err := ctx.Err(); err != nil {
		summary.Err = err
		return summary
	}
	ids, err := s.data.Instruments(ctx, s.scope)
	if err != nil {
		summary.Err = err
		return summary
	}
	if len(ids) > s.scope.Limit {
		summary.Err = invalidRequest("instrument response exceeds scope")
		return summary
	}
	unique := map[market.InstrumentID]bool{}
	jobs := make(chan market.InstrumentID)
	type outcome struct {
		id     market.InstrumentID
		result RefreshResult
		err    error
	}
	// 有界结果缓冲仅与 worker 数量相关，主 goroutine 始终负责归集。
	outcomes := make(chan outcome, workers)
	queue := make([]market.InstrumentID, 0, len(ids))
	for _, id := range ids {
		if err := id.Validate(); err != nil {
			summary.Err = invalidRequest("invalid universe instrument")
			return summary
		}
		if !unique[id] {
			unique[id] = true
			queue = append(queue, id)
		}
	}
	summary.Total = len(queue)
	observation.Prepared(summary.Total)
	if len(queue) == 0 {
		return summary
	}
	if workers > len(queue) {
		workers = len(queue)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				var result RefreshResult
				err := ctx.Err()
				if err == nil {
					result, err = s.refresher.Refresh(ctx, id)
				}
				outcomes <- outcome{id, result, err}
			}
		}()
	}
	// 仅一个派发者与一个关闭者，取消后仍归集全部已接收工作并等待 worker 退出。
	go func() {
		defer close(jobs)
		for _, id := range queue {
			if ctx.Err() != nil {
				return
			}
			select {
			case jobs <- id:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(outcomes) }()
	for item := range outcomes {
		observation.Completed(item.id, item.err)
		if item.err != nil {
			summary.Failures[item.id] = item.err
		} else {
			summary.Results[item.id] = item.result
		}
	}
	if err := ctx.Err(); err != nil {
		summary.Err = err
		for _, id := range queue {
			if _, ok := summary.Results[id]; !ok && summary.Failures[id] == nil {
				summary.Failures[id] = err
			}
		}
	}
	return summary
}

// Start 同步运行到取消；调用方可用一个受管理的 goroutine 启动，并在关闭数据库前等待返回。
// 首次等待 interval 到达后执行。重复 Start 不会留下多余 ticker；证券级失败保存在 LastSummary。
func (s *MarketScheduler) Start(ctx context.Context, interval time.Duration, workers int) error {
	if interval <= 0 || workers < 1 || workers > MaxMarketWorkers {
		return invalidRequest("invalid scheduler interval or workers")
	}
	if !s.started.CompareAndSwap(false, true) {
		return ErrRefreshAlreadyRunning
	}
	defer s.started.Store(false)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		summary := s.RunOnce(ctx, workers)
		if summary.Err != nil && !errors.Is(summary.Err, ErrRefreshAlreadyRunning) {
			return summary.Err
		}
	}
}
func (s *MarketScheduler) LastSummary() RefreshSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRefreshSummary(s.last)
}
func cloneRefreshSummary(input RefreshSummary) RefreshSummary {
	result := input
	result.Results = make(map[market.InstrumentID]RefreshResult, len(input.Results))
	for id, r := range input.Results {
		result.Results[id] = r
	}
	result.Failures = make(map[market.InstrumentID]error, len(input.Failures))
	for id, e := range input.Failures {
		result.Failures[id] = e
	}
	return result
}
