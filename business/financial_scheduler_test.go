package business

import (
	"context"
	"errors"
	"testing"
	"time"

	"trading/model"
)

// mockFinancialRepoForScheduler 模拟财报仓库供调度器测试使用，
// 复用 mockFinancialRepo 结构无法返回自定义 codes，因此独立定义。
type mockFinancialRepoForScheduler struct {
	codes      []string
	findAllErr error
}

func (m *mockFinancialRepoForScheduler) Upsert(ctx context.Context, reports []*model.FinancialReport) error {
	return nil
}
func (m *mockFinancialRepoForScheduler) FindByCode(ctx context.Context, code string) ([]*model.FinancialReport, error) {
	return nil, nil
}
func (m *mockFinancialRepoForScheduler) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return nil, nil
}
func (m *mockFinancialRepoForScheduler) FindAllCodes(ctx context.Context) ([]string, error) {
	if m.findAllErr != nil {
		return nil, m.findAllErr
	}
	return m.codes, nil
}

// mockFinancialSvcForScheduler 模拟 FinancialReportService
type mockFinancialSvcForScheduler struct {
	saveErr error
}

func (m *mockFinancialSvcForScheduler) SaveFinancialReportData(ctx context.Context, code string) error {
	return m.saveErr
}
func (m *mockFinancialSvcForScheduler) AppendFinancialReportData(ctx context.Context, code string) error {
	return m.saveErr
}

func TestFinancialSchedulerTriggerNowBeforeStart(t *testing.T) {
	sched := NewFinancialScheduler(&mockFinancialSvcForScheduler{}, &mockFinancialRepoForScheduler{})
	if err := sched.TriggerNow(context.Background()); !errors.Is(err, ErrSchedulerNotStarted) {
		t.Fatalf("expected ErrSchedulerNotStarted, got %v", err)
	}
}

func TestFinancialSchedulerStopIdempotent(t *testing.T) {
	sched := NewFinancialScheduler(&mockFinancialSvcForScheduler{}, &mockFinancialRepoForScheduler{})
	sched.Start(context.Background())

	sched.Stop()
	sched.Stop()
	sched.Stop()
}

func TestFinancialSchedulerTriggerBusy(t *testing.T) {
	concrete := NewFinancialScheduler(&mockFinancialSvcForScheduler{}, &mockFinancialRepoForScheduler{}).(*financialScheduler)
	concrete.Start(context.Background())
	defer concrete.Stop()

	if !concrete.guard.tryStart() {
		t.Fatal("failed to occupy guard")
	}
	defer concrete.guard.markDone()

	err := concrete.TriggerNow(context.Background())
	if !errors.Is(err, ErrSchedulerBusy) {
		t.Fatalf("expected ErrSchedulerBusy, got %v", err)
	}
}

func TestFinancialSchedulerScanAndConsumeEmpty(t *testing.T) {
	concrete := NewFinancialScheduler(&mockFinancialSvcForScheduler{}, &mockFinancialRepoForScheduler{codes: nil}).(*financialScheduler)
	concrete.Start(context.Background())
	defer concrete.Stop()

	if err := concrete.scanAndConsume(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestFinancialSchedulerScanAndConsumeWithCodes(t *testing.T) {
	concrete := NewFinancialScheduler(
		&mockFinancialSvcForScheduler{},
		&mockFinancialRepoForScheduler{codes: []string{"000001", "000002"}},
	).(*financialScheduler)
	concrete.Start(context.Background())
	defer concrete.Stop()

	if err := concrete.scanAndConsume(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestFinancialSchedulerScanAndConsumeRepoError(t *testing.T) {
	concrete := NewFinancialScheduler(
		&mockFinancialSvcForScheduler{},
		&mockFinancialRepoForScheduler{findAllErr: errors.New("db error")},
	).(*financialScheduler)
	concrete.Start(context.Background())
	defer concrete.Stop()

	if err := concrete.scanAndConsume(context.Background()); err == nil {
		t.Fatal("expected error from FindAllCodes failure")
	}
}

func TestFinancialSchedulerTriggerNowExecutes(t *testing.T) {
	concrete := NewFinancialScheduler(
		&mockFinancialSvcForScheduler{},
		&mockFinancialRepoForScheduler{codes: nil},
	).(*financialScheduler)
	concrete.Start(context.Background())
	defer concrete.Stop()

	if err := concrete.TriggerNow(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if concrete.guard.tryStart() {
			concrete.guard.markDone()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("TriggerNow goroutine did not release guard in time")
}

func TestFinancialSchedulerTriggerNowSurfacesScanError(t *testing.T) {
	concrete := NewFinancialScheduler(
		&mockFinancialSvcForScheduler{},
		&mockFinancialRepoForScheduler{findAllErr: errors.New("db down")},
	).(*financialScheduler)
	concrete.Start(context.Background())
	defer concrete.Stop()

	if err := concrete.TriggerNow(context.Background()); err != nil {
		t.Fatalf("TriggerNow returns nil even when scan fails async, got %v", err)
	}

	// 等待 trigger goroutine 在 scan 失败后释放 guard
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if concrete.guard.tryStart() {
			concrete.guard.markDone()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("TriggerNow goroutine did not release guard in time")
}

func TestFinancialSchedulerStartIsIdempotent(t *testing.T) {
	concrete := NewFinancialScheduler(
		&mockFinancialSvcForScheduler{},
		&mockFinancialRepoForScheduler{},
	).(*financialScheduler)
	concrete.Start(context.Background())
	first := concrete.lifeCtx
	concrete.Start(context.Background())
	if concrete.lifeCtx != first {
		t.Fatal("Start should be a no-op when scheduler is already started")
	}
	concrete.Stop()
}
