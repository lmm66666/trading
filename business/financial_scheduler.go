package business

import (
	"context"
	"fmt"
	"log"

	"trading/data"
)

// FinancialScheduler 财报调度器接口
type FinancialScheduler interface {
	TriggerNow(ctx context.Context) error
}

type financialScheduler struct {
	svc           FinancialReportService
	financialRepo data.FinancialReportRepo
	guard         triggerGuard
	worker        *concurrentWorker
}

// NewFinancialScheduler 创建 FinancialScheduler 实例
func NewFinancialScheduler(svc FinancialReportService, financialRepo data.FinancialReportRepo) FinancialScheduler {
	return &financialScheduler{
		svc:           svc,
		financialRepo: financialRepo,
		worker:        newConcurrentWorker(100),
	}
}

// TriggerNow 手动触发一次财报扫描（供 API 调用）
func (s *financialScheduler) TriggerNow(ctx context.Context) error {
	if !s.guard.tryStart() {
		return fmt.Errorf("another task is already running")
	}

	log.Println("[financial-scheduler] manual trigger started")
	go func() {
		defer s.guard.markDone()
		if err := s.scanAndConsume(context.Background()); err != nil {
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
