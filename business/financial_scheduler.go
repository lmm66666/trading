package business

import (
	"context"
	"fmt"
	"log"
	"sync"

	"trading/data"
)

// FinancialScheduler 财报调度器接口
type FinancialScheduler interface {
	Start(ctx context.Context)
	Stop()
	TriggerNow(ctx context.Context) error
}

type financialScheduler struct {
	svc           FinancialReportService
	financialRepo data.FinancialReportRepo
	guard         triggerGuard
	worker        *concurrentWorker

	mu         sync.Mutex
	lifeCtx    context.Context
	lifeCancel context.CancelFunc
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// NewFinancialScheduler 创建 FinancialScheduler 实例
func NewFinancialScheduler(svc FinancialReportService, financialRepo data.FinancialReportRepo) FinancialScheduler {
	return &financialScheduler{
		svc:           svc,
		financialRepo: financialRepo,
		worker:        newConcurrentWorker(100),
	}
}

// Start 启动调度器并保存生命周期 ctx，供 TriggerNow 的后台任务使用
func (s *financialScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifeCtx != nil {
		return
	}
	s.lifeCtx, s.lifeCancel = context.WithCancel(ctx)
}

// Stop 停止调度器；可重复调用（幂等），并等待所有后台 goroutine 退出
func (s *financialScheduler) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel := s.lifeCancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})
	s.wg.Wait()
}

// TriggerNow 手动触发一次财报扫描（供 API 调用）
func (s *financialScheduler) TriggerNow(_ context.Context) error {
	s.mu.Lock()
	lifeCtx := s.lifeCtx
	s.mu.Unlock()
	if lifeCtx == nil {
		return ErrSchedulerNotStarted
	}
	if !s.guard.tryStart() {
		return ErrSchedulerBusy
	}

	log.Println("[financial-scheduler] manual trigger started")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.guard.markDone()
		if err := s.scanAndConsume(lifeCtx); err != nil {
			log.Printf("[financial-scheduler] scan failed: %v", err)
		}
	}()

	return nil
}

func (s *financialScheduler) scanAndConsume(ctx context.Context) error {
	codes, err := s.financialRepo.FindAllCodes(ctx)
	if err != nil {
		return fmt.Errorf("find all codes failed: %w", err)
	}

	if len(codes) == 0 {
		log.Println("[financial-scheduler] no codes found")
		return nil
	}

	log.Printf("[financial-scheduler] %d codes queued", len(codes))

	errs := s.worker.run(ctx, codes, func(ctx context.Context, code string) error {
		return s.svc.AppendFinancialReportData(ctx, code)
	})

	for _, err := range errs {
		log.Printf("[financial-scheduler] failed: %v", err)
	}

	log.Printf("[financial-scheduler] all tasks done, %d failed", len(errs))
	return nil
}
