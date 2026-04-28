package business

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"trading/data"
)

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
	stopCh     chan struct{}
	interval   time.Duration
	guard      triggerGuard
	worker     *concurrentWorker
	bgCtx      context.Context
}

// NewScheduler 创建 Scheduler 实例
func NewScheduler(svc StockDataService, dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo) Scheduler {
	return &stockScheduler{
		svc:        svc,
		dailyRepo:  dailyRepo,
		weeklyRepo: weeklyRepo,
		stopCh:     make(chan struct{}),
		interval:   5 * time.Second,
		worker:     newConcurrentWorker(100),
	}
}

// Start 启动调度器，按指定时间每天执行扫描
func (s *stockScheduler) Start(ctx context.Context, hour, minute int) {
	s.bgCtx = ctx
	go s.run(ctx, hour, minute)
}

// Stop 停止调度器
func (s *stockScheduler) Stop() {
	close(s.stopCh)
}

func (s *stockScheduler) run(ctx context.Context, hour, minute int) {
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
			if s.guard.isRunning() {
				log.Println("[scheduler] skipped: manual task is running")
				continue
			}
			s.scanAndConsume(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// TriggerNow 手动触发一次扫描（供 API 调用）
func (s *stockScheduler) TriggerNow(ctx context.Context) error {
	if !s.guard.tryStart() {
		return fmt.Errorf("another task is already running")
	}

	log.Println("[scheduler] manual trigger started")
	go func() {
		defer s.guard.markDone()
		s.scanAndConsume(s.bgCtx)
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
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		log.Printf("[scheduler] [%d/%d] processing %s (daily=%v weekly=%v)", i+1, len(tasks), t.code, t.needDaily, t.needWeekly)
		if err = s.process(ctx, t); err != nil {
			log.Printf("[scheduler] [%d/%d] failed %s: %v", i+1, len(tasks), t.code, err)
		} else {
			log.Printf("[scheduler] [%d/%d] success %s", i+1, len(tasks), t.code)
		}

		if i < len(tasks)-1 {
			time.Sleep(s.interval)
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
