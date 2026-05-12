package business

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"trading/pkg/indicator"
)

// triggerGuard provides run-once protection for manual triggers.
type triggerGuard struct {
	running atomic.Bool
}

func (tg *triggerGuard) tryStart() bool {
	return tg.running.CompareAndSwap(false, true)
}

func (tg *triggerGuard) markDone() {
	tg.running.Store(false)
}

// concurrentWorker executes a handler over a slice of items with a limiter.
type concurrentWorker struct {
	limiter *indicator.Limiter
}

func newConcurrentWorker(maxConcurrent int) *concurrentWorker {
	return &concurrentWorker{limiter: indicator.NewLimiter(maxConcurrent)}
}

func (cw *concurrentWorker) run(ctx context.Context, items []string, handler func(ctx context.Context, item string) error) []error {
	type result struct {
		code string
		err  error
	}

	results := make(chan result, len(items))
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			if err := cw.limiter.Acquire(ctx); err != nil {
				results <- result{code: c, err: fmt.Errorf("limiter acquire failed: %w", err)}
				return
			}
			defer cw.limiter.Release()

			if err := handler(ctx, c); err != nil {
				results <- result{code: c, err: err}
			}
		}(item)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var errs []error
	for r := range results {
		errs = append(errs, fmt.Errorf("%s: %w", r.code, r.err))
	}
	return errs
}
