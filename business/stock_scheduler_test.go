package business

import (
	"context"
	"errors"
	"testing"
	"time"

	"trading/model"
)

// mockDailyRepoForScheduler 模拟日线数据仓库（支持 FindLatestByCode / FindAllCodes）
type mockDailyRepoForScheduler struct {
	codes       []string
	latest      map[string]*model.StockKlineDaily
	latestErr   map[string]error
	findAllErr  error
}

func (m *mockDailyRepoForScheduler) Create(ctx context.Context, kline *model.StockKlineDaily) error         { return nil }
func (m *mockDailyRepoForScheduler) CreateBatch(ctx context.Context, klines []*model.StockKlineDaily) error { return nil }
func (m *mockDailyRepoForScheduler) Upsert(ctx context.Context, klines []*model.StockKlineDaily) error      { return nil }
func (m *mockDailyRepoForScheduler) FindByID(ctx context.Context, id uint) (*model.StockKlineDaily, error)  { return nil, nil }
func (m *mockDailyRepoForScheduler) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockDailyRepoForScheduler) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockDailyRepoForScheduler) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineDaily, error) {
	if err, ok := m.latestErr[code]; ok {
		return nil, err
	}
	if k, ok := m.latest[code]; ok {
		return k, nil
	}
	return nil, errors.New("not found")
}
func (m *mockDailyRepoForScheduler) FindAllCodes(ctx context.Context) ([]string, error) {
	if m.findAllErr != nil {
		return nil, m.findAllErr
	}
	return m.codes, nil
}
func (m *mockDailyRepoForScheduler) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockDailyRepoForScheduler) Update(ctx context.Context, kline *model.StockKlineDaily) error { return nil }
func (m *mockDailyRepoForScheduler) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockDailyRepoForScheduler) List(ctx context.Context, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}

// mockWeeklyRepoForScheduler 模拟周线数据仓库（支持 FindLatestByCode / FindAllCodes）
type mockWeeklyRepoForScheduler struct {
	codes       []string
	latest      map[string]*model.StockKlineWeekly
	latestErr   map[string]error
	findAllErr  error
}

func (m *mockWeeklyRepoForScheduler) Create(ctx context.Context, kline *model.StockKlineWeekly) error         { return nil }
func (m *mockWeeklyRepoForScheduler) CreateBatch(ctx context.Context, klines []*model.StockKlineWeekly) error { return nil }
func (m *mockWeeklyRepoForScheduler) Upsert(ctx context.Context, klines []*model.StockKlineWeekly) error      { return nil }
func (m *mockWeeklyRepoForScheduler) FindByID(ctx context.Context, id uint) (*model.StockKlineWeekly, error)  { return nil, nil }
func (m *mockWeeklyRepoForScheduler) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockWeeklyRepoForScheduler) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockWeeklyRepoForScheduler) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineWeekly, error) {
	if err, ok := m.latestErr[code]; ok {
		return nil, err
	}
	if k, ok := m.latest[code]; ok {
		return k, nil
	}
	return nil, errors.New("not found")
}
func (m *mockWeeklyRepoForScheduler) FindAllCodes(ctx context.Context) ([]string, error) {
	if m.findAllErr != nil {
		return nil, m.findAllErr
	}
	return m.codes, nil
}
func (m *mockWeeklyRepoForScheduler) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockWeeklyRepoForScheduler) Update(ctx context.Context, kline *model.StockKlineWeekly) error { return nil }
func (m *mockWeeklyRepoForScheduler) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockWeeklyRepoForScheduler) List(ctx context.Context, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}

// mockSvcForScheduler 模拟 StockDataService
type mockSvcForScheduler struct {
	saveErr error
}

func (m *mockSvcForScheduler) SaveHistoricalData(ctx context.Context, code string) error {
	return m.saveErr
}
func (m *mockSvcForScheduler) AppendStockData(ctx context.Context, code string, daily, weekly bool) error {
	return m.saveErr
}

func TestLastFridayDate(t *testing.T) {
	tests := []struct {
		input    time.Time
		expected string
	}{
		{time.Date(2025, 4, 21, 0, 0, 0, 0, time.UTC), "2025-04-18"}, // Monday -> last Friday
		{time.Date(2025, 4, 22, 0, 0, 0, 0, time.UTC), "2025-04-18"}, // Tuesday
		{time.Date(2025, 4, 23, 0, 0, 0, 0, time.UTC), "2025-04-18"}, // Wednesday
		{time.Date(2025, 4, 24, 0, 0, 0, 0, time.UTC), "2025-04-18"}, // Thursday
		{time.Date(2025, 4, 25, 0, 0, 0, 0, time.UTC), "2025-04-25"}, // Friday -> today
		{time.Date(2025, 4, 26, 0, 0, 0, 0, time.UTC), "2025-04-25"}, // Saturday -> last Friday
		{time.Date(2025, 4, 27, 0, 0, 0, 0, time.UTC), "2025-04-25"}, // Sunday -> last Friday
	}

	for _, tt := range tests {
		got := lastFridayDate(tt.input)
		if got != tt.expected {
			t.Errorf("lastFridayDate(%v) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestSchedulerScanAllUpToDate(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	lastFriday := lastFridayDate(time.Now())

	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: today},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineWeekly{
			"000001": {Code: "000001", Date: lastFriday},
		},
	}

	svc := &mockSvcForScheduler{}
	sched := NewScheduler(svc, dailyRepo, weeklyRepo).(*stockScheduler)

	tasks, err := sched.scan(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestSchedulerScanMissingDaily(t *testing.T) {
	lastFriday := lastFridayDate(time.Now())

	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: "2020-01-01"},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineWeekly{
			"000001": {Code: "000001", Date: lastFriday},
		},
	}

	svc := &mockSvcForScheduler{}
	sched := NewScheduler(svc, dailyRepo, weeklyRepo).(*stockScheduler)

	tasks, err := sched.scan(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if !tasks[0].needDaily {
		t.Fatal("expected needDaily to be true")
	}
	if tasks[0].needWeekly {
		t.Fatal("expected needWeekly to be false")
	}
}

func TestSchedulerScanMissingWeekly(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: today},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineWeekly{
			"000001": {Code: "000001", Date: "2020-01-01"},
		},
	}

	svc := &mockSvcForScheduler{}
	sched := NewScheduler(svc, dailyRepo, weeklyRepo).(*stockScheduler)

	tasks, err := sched.scan(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].needDaily {
		t.Fatal("expected needDaily to be false")
	}
	if !tasks[0].needWeekly {
		t.Fatal("expected needWeekly to be true")
	}
}

func TestSchedulerScanUnionCodes(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	lastFriday := lastFridayDate(time.Now())

	// daily has code A, weekly has code B -> union should include both
	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: today},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes: []string{"000002"},
		latest: map[string]*model.StockKlineWeekly{
			"000002": {Code: "000002", Date: lastFriday},
		},
	}

	svc := &mockSvcForScheduler{}
	sched := NewScheduler(svc, dailyRepo, weeklyRepo).(*stockScheduler)

	tasks, err := sched.scan(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// 000001 missing in weekly, 000002 missing in daily -> 2 tasks
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestSchedulerProcessSuccess(t *testing.T) {
	svc := &mockSvcForScheduler{}
	sched := NewScheduler(svc, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{}).(*stockScheduler)

	err := sched.process(context.Background(), task{code: "000001", needDaily: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSchedulerProcessError(t *testing.T) {
	svc := &mockSvcForScheduler{saveErr: errors.New("save failed")}
	sched := NewScheduler(svc, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{}).(*stockScheduler)

	err := sched.process(context.Background(), task{code: "000001", needDaily: true})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSchedulerTriggerNowBeforeStart(t *testing.T) {
	sched := NewScheduler(&mockSvcForScheduler{}, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{})
	if err := sched.TriggerNow(context.Background()); !errors.Is(err, ErrSchedulerNotStarted) {
		t.Fatalf("expected ErrSchedulerNotStarted, got %v", err)
	}
}

func TestSchedulerStopIdempotent(t *testing.T) {
	sched := NewScheduler(&mockSvcForScheduler{}, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{}).(*stockScheduler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx, 23, 59)

	// 多次调用 Stop 不应 panic
	sched.Stop()
	sched.Stop()
	sched.Stop()
}

func TestSchedulerTriggerBusyReturnsErrSchedulerBusy(t *testing.T) {
	sched := NewScheduler(&mockSvcForScheduler{}, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{}).(*stockScheduler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx, 23, 59)
	defer sched.Stop()

	// 占用 guard 模拟已有任务运行
	if !sched.guard.tryStart() {
		t.Fatal("failed to occupy guard")
	}
	defer sched.guard.markDone()

	err := sched.TriggerNow(context.Background())
	if !errors.Is(err, ErrSchedulerBusy) {
		t.Fatalf("expected ErrSchedulerBusy, got %v", err)
	}
}

func TestSchedulerStopCancelsLifeCtx(t *testing.T) {
	sched := NewScheduler(&mockSvcForScheduler{}, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{}).(*stockScheduler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx, 23, 59)

	sched.Stop()

	select {
	case <-sched.lifeCtx.Done():
	default:
		t.Fatal("expected lifeCtx to be cancelled after Stop")
	}
}

func TestSchedulerScanAndConsumeNoTasks(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	lastFriday := lastFridayDate(time.Now())

	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: today},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineWeekly{
			"000001": {Code: "000001", Date: lastFriday},
		},
	}

	sched := NewScheduler(&mockSvcForScheduler{}, dailyRepo, weeklyRepo).(*stockScheduler)
	// 直接调用 scanAndConsume 不需要 Start
	sched.scanAndConsume(context.Background())
}

func TestSchedulerScanAndConsumeWithTasks(t *testing.T) {
	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: "2020-01-01"},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes:  []string{"000001"},
		latest: map[string]*model.StockKlineWeekly{},
	}

	sched := NewScheduler(&mockSvcForScheduler{}, dailyRepo, weeklyRepo).(*stockScheduler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.scanAndConsume(ctx)
}

func TestSchedulerScanAndConsumeCancelStops(t *testing.T) {
	dailyRepo := &mockDailyRepoForScheduler{
		codes: []string{"000001", "000002", "000003"},
		latest: map[string]*model.StockKlineDaily{
			"000001": {Code: "000001", Date: "2020-01-01"},
			"000002": {Code: "000002", Date: "2020-01-01"},
			"000003": {Code: "000003", Date: "2020-01-01"},
		},
	}
	weeklyRepo := &mockWeeklyRepoForScheduler{
		codes:  []string{"000001"},
		latest: map[string]*model.StockKlineWeekly{},
	}

	sched := NewScheduler(&mockSvcForScheduler{}, dailyRepo, weeklyRepo).(*stockScheduler)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，scanAndConsume 应当尽快退出

	sched.scanAndConsume(ctx)
}

func TestSchedulerTriggerNowExecutes(t *testing.T) {
	dailyRepo := &mockDailyRepoForScheduler{codes: nil}
	weeklyRepo := &mockWeeklyRepoForScheduler{codes: nil}
	sched := NewScheduler(&mockSvcForScheduler{}, dailyRepo, weeklyRepo).(*stockScheduler)
	sched.Start(context.Background(), 23, 59)
	defer sched.Stop()

	if err := sched.TriggerNow(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// 等待 trigger goroutine 完成（通过 guard.tryStart 释放检测）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sched.guard.tryStart() {
			sched.guard.markDone()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("TriggerNow goroutine did not release guard in time")
}

func TestSchedulerStartIsIdempotent(t *testing.T) {
	sched := NewScheduler(&mockSvcForScheduler{}, &mockDailyRepoForScheduler{}, &mockWeeklyRepoForScheduler{}).(*stockScheduler)
	sched.Start(context.Background(), 23, 59)
	first := sched.lifeCtx
	sched.Start(context.Background(), 23, 59) // 第二次调用应当无副作用
	if sched.lifeCtx != first {
		t.Fatal("Start should be a no-op when scheduler is already started")
	}
	sched.Stop()
}
