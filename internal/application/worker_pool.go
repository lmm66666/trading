package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"trading/internal/port"
)

type RunHandler func(context.Context, port.Run) error
type WorkerPoolConfig struct {
	Owner               string
	Workers             int
	Lease, PollInterval time.Duration
	Clock               func() time.Time
	Telemetry           port.Telemetry
}
type WorkerPool struct {
	queue    port.JobQueue
	store    port.RunStore
	handlers map[port.RunKind]RunHandler
	config   WorkerPoolConfig
	running  atomic.Bool
}

func NewWorkerPool(queue port.JobQueue, store port.RunStore, handlers map[port.RunKind]RunHandler, config WorkerPoolConfig) (*WorkerPool, error) {
	if nilComputeDependency(queue) || nilComputeDependency(store) || config.Workers < 1 || config.Workers > 64 || config.Lease < 3*time.Millisecond || config.PollInterval <= 0 || len(handlers) == 0 {
		return nil, invalidRequest("invalid worker pool configuration")
	}
	if err := port.ValidateIdentity(config.Owner, "worker owner", port.MaxLeaseIdentityBytes, false); err != nil {
		return nil, err
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	copied := make(map[port.RunKind]RunHandler, len(handlers))
	for kind, handler := range handlers {
		if kind.Validate() != nil || handler == nil {
			return nil, invalidRequest("invalid run handler")
		}
		copied[kind] = handler
	}
	return &WorkerPool{queue: queue, store: store, handlers: copied, config: config}, nil
}

// Run owns all worker/renewal goroutines and waits for them before returning.
// Storage and handlers must honor context; shutdown leaves live leases for
// durable reclamation instead of incorrectly terminalizing interrupted work.
func (p *WorkerPool) Run(ctx context.Context) error {
	if !p.running.CompareAndSwap(false, true) {
		return invalidRequest("worker pool already running")
	}
	defer p.running.Store(false)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if _, err := p.queue.ReapExpired(ctx); err != nil {
		return err
	}
	errs := make(chan error, p.config.Workers)
	var wg sync.WaitGroup
	for i := 0; i < p.config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.work(ctx); err != nil {
				select {
				case errs <- err:
				default:
				}
				cancel()
			}
		}()
	}
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()
	var result error
loop:
	for {
		select {
		case <-ctx.Done():
			result = ctx.Err()
			break loop
		case err := <-errs:
			result = err
			break loop
		case <-ticker.C:
			if _, err := p.queue.ReapExpired(ctx); err != nil {
				result = err
				break loop
			}
		}
	}
	cancel()
	wg.Wait()
	select {
	case err := <-errs:
		if errors.Is(result, context.Canceled) {
			result = err
		}
	default:
	}
	return result
}
func (p *WorkerPool) work(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := p.config.Clock()
		run, err := p.queue.Claim(ctx, p.config.Owner, p.config.Lease)
		if errors.Is(err, port.ErrRunNotFound) {
			timer := time.NewTimer(p.config.PollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if err != nil {
			return err
		}
		ComputeConfig{Clock: p.config.Clock, Telemetry: p.config.Telemetry}.observe(ctx, run, "queue_wait", start, 0, 0, 0, 0)
		if err = p.process(ctx, run); err != nil {
			return err
		}
	}
}
func (p *WorkerPool) process(ctx context.Context, run port.Run) error {
	executionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	watchDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(p.config.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				watchDone <- nil
				return
			case <-executionCtx.Done():
				watchDone <- executionCtx.Err()
				return
			case <-ticker.C:
				err := checkRun(executionCtx, p.store, run)
				if err == nil {
					err = p.queue.Renew(executionCtx, run.ID, run.LeaseToken, p.config.Lease)
				}
				if err != nil {
					cancel()
					watchDone <- err
					return
				}
			}
		}
	}()
	handler, ok := p.handlers[run.Kind]
	var err error
	if !ok {
		err = invalidRequest("unsupported run kind")
	} else {
		err = handler(executionCtx, run)
	}
	close(stop)
	watchErr := <-watchDone
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if watchErr != nil {
		if errors.Is(watchErr, port.ErrLeaseLost) || errors.Is(watchErr, context.Canceled) {
			return nil
		}
		return watchErr
	}
	if errors.Is(err, port.ErrLeaseLost) || errors.Is(err, context.Canceled) {
		return nil
	}
	failure := classifyFailure(err)
	if at, ok := retryAt(p.config.Clock(), run.Attempts); ok && failure.Retryable {
		if err = p.queue.Retry(ctx, run.ID, run.LeaseToken, at, failure); err != nil {
			if errors.Is(err, port.ErrLeaseLost) {
				return nil
			}
			return err
		}
		if !nilComputeDependency(p.config.Telemetry) {
			p.config.Telemetry.CountRetry(ctx, run.ID, failure.Code, run.Attempts)
		}
		return nil
	}
	failure.Retryable = false
	err = p.store.Fail(ctx, run.ID, run.LeaseToken, failure)
	if errors.Is(err, port.ErrLeaseLost) {
		return nil
	}
	return err
}
