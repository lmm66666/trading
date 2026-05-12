package business

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"trading/data"
)

// ErrSchedulerNotStarted 调度器未启动时手动触发返回此错误
var ErrSchedulerNotStarted = errors.New("scheduler not started")

// ErrSchedulerBusy 已有任务在运行时再次触发返回此错误
var ErrSchedulerBusy = errors.New("another task is already running")

// Scheduler 调度器接口
type Scheduler interface {
	Start(ctx context.Context, hour, minute int)
	Stop()
	TriggerNow(ctx context.Context) error
}

// stockScheduler 定时任务调度器实现，每天扫描并补充缺失的股票数据
type stockScheduler struct {
	svc        StockDataService
	dailyRepo  data.StockKlineDailyRepo
	weeklyRepo data.StockKlineWeeklyRepo
	interval   time.Duration
	guard      triggerGuard
	worker     *concurrentWorker

	mu          sync.Mutex
	lifeCtx     context.Context
	lifeCancel  context.CancelFunc
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

// NewScheduler 创建 Scheduler 实例
func NewScheduler(svc StockDataService, dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo) Scheduler {
	return &stockScheduler{
		svc:        svc,
		dailyRepo:  dailyRepo,
		weeklyRepo: weeklyRepo,
		interval:   5 * time.Second,
		worker:     newConcurrentWorker(100),
	}
}

// Start 启动调度器，按指定时间每天执行扫描
func (s *stockScheduler) Start(ctx context.Context, hour, minute int) {
	s.mu.Lock()
	if s.lifeCtx != nil {
		s.mu.Unlock()
		return
	}
	s.lifeCtx, s.lifeCancel = context.WithCancel(ctx)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(hour, minute)
}

// Stop 停止调度器；可重复调用（幂等），并等待所有后台 goroutine 退出
func (s *stockScheduler) Stop() {
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

func (s *stockScheduler) run(hour, minute int) {
	defer s.wg.Done()
	for {
		nextRun := nextDailyTime(hour, minute)
		wait := time.Until(nextRun)
		log.Printf("[scheduler] next scan at %v (in %v)", nextRun.Format("2006-01-02 15:04:05"), wait)

		select {
		case <-time.After(wait):
			if !isWeekday(time.Now()) {
				log.Println("[scheduler] skipped: not a weekday")
				continue
			}
			if !s.guard.tryStart() {
				log.Println("[scheduler] skipped: manual task is running")
				continue
			}
			s.scanAndConsume(s.lifeCtx)
			s.guard.markDone()
		case <-s.lifeCtx.Done():
			return
		}
	}
}

// TriggerNow 手动触发一次扫描（供 API 调用）
func (s *stockScheduler) TriggerNow(_ context.Context) error {
	s.mu.Lock()
	lifeCtx := s.lifeCtx
	s.mu.Unlock()
	if lifeCtx == nil {
		return ErrSchedulerNotStarted
	}
	if !s.guard.tryStart() {
		return ErrSchedulerBusy
	}

	log.Println("[scheduler] manual trigger started")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.guard.markDone()
		s.scanAndConsume(lifeCtx)
	}()
	return nil
}

func (s *stockScheduler) scanAndConsume(ctx context.Context) {
	tasks, err := s.scan(ctx)
	if err != nil {
		log.Printf("[scheduler] scan failed: %v", err)
		return
	}

	if len(tasks) == 0 {
		log.Println("[scheduler] no missing data, all up to date")
		return
	}

	log.Printf("[scheduler] %d tasks queued, consuming one every %v", len(tasks), s.interval)

	for i, t := range tasks {
		if err := ctx.Err(); err != nil {
			return
		}

		log.Printf("[scheduler] [%d/%d] processing %s (daily=%v weekly=%v)", i+1, len(tasks), t.code, t.needDaily, t.needWeekly)
		if err := s.process(ctx, t); err != nil {
			log.Printf("[scheduler] [%d/%d] failed %s: %v", i+1, len(tasks), t.code, err)
		} else {
			log.Printf("[scheduler] [%d/%d] success %s", i+1, len(tasks), t.code)
		}

		if i < len(tasks)-1 {
			select {
			case <-time.After(s.interval):
			case <-ctx.Done():
				return
			}
		}
	}

	log.Println("[scheduler] all tasks done")
}

// findCodes returns the union of daily and weekly codes.
func (s *stockScheduler) findCodes(ctx context.Context) ([]string, error) {
	dailyCodes, err := s.dailyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find daily codes failed: %w", err)
	}

	weeklyCodes, err := s.weeklyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find weekly codes failed: %w", err)
	}

	codeSet := make(map[string]struct{})
	for _, c := range dailyCodes {
		codeSet[c] = struct{}{}
	}
	for _, c := range weeklyCodes {
		codeSet[c] = struct{}{}
	}

	codes := make([]string, 0, len(codeSet))
	for c := range codeSet {
		codes = append(codes, c)
	}

	return codes, nil
}

// concurrentCheckCodes checks all codes concurrently and returns tasks needing updates.
func (s *stockScheduler) concurrentCheckCodes(ctx context.Context, codes []string) ([]task, []error) {
	today := time.Now().Format("2006-01-02")
	lastFriday := lastFridayDate(time.Now())

	var mu sync.Mutex
	var tasks []task

	errs := s.worker.run(ctx, codes, func(ctx context.Context, code string) error {
		needDaily, needWeekly, checkErr := s.checkCode(ctx, code, today, lastFriday)
		if checkErr != nil {
			return checkErr
		}
		if needDaily || needWeekly {
			mu.Lock()
			tasks = append(tasks, task{code: code, needDaily: needDaily, needWeekly: needWeekly})
			mu.Unlock()
		}
		return nil
	})

	return tasks, errs
}

// scan 扫描所有股票代码，返回需要更新的任务列表
func (s *stockScheduler) scan(ctx context.Context) ([]task, error) {
	codes, err := s.findCodes(ctx)
	if err != nil {
		return nil, err
	}

	if len(codes) == 0 {
		return nil, nil
	}

	tasks, errs := s.concurrentCheckCodes(ctx, codes)
	if len(errs) > 0 {
		for _, e := range errs {
			log.Printf("[scheduler] check failed: %v", e)
		}
	}

	return tasks, nil
}

func (s *stockScheduler) checkCode(ctx context.Context, code, today, lastFriday string) (bool, bool, error) {
	var needDaily bool
	latestDaily, err := s.dailyRepo.FindLatestByCode(ctx, code)
	if err != nil {
		needDaily = true
	} else if latestDaily.Date != today {
		needDaily = true
	}

	var needWeekly bool
	latestWeekly, err := s.weeklyRepo.FindLatestByCode(ctx, code)
	if err != nil {
		needWeekly = true
	} else if latestWeekly.Date != lastFriday {
		needWeekly = true
	}

	return needDaily, needWeekly, nil
}

func (s *stockScheduler) process(ctx context.Context, t task) error {
	if t.needDaily || t.needWeekly {
		return s.svc.AppendStockData(ctx, t.code)
	}
	return nil
}

type task struct {
	code       string
	needDaily  bool
	needWeekly bool
}

// nextDailyTime 计算下一个指定时间（今天或明天）
func nextDailyTime(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
