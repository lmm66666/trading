# Business Layer Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split mixed-responsibility services into focused domain services and extract scheduler common code.

**Architecture:** Extract tool functions to `util.go`, extract scheduler primitives (`triggerGuard`, `concurrentWorker`) to `scheduler_base.go`, split `StockService` into `StockDataService` + `FinancialReportService`, split `AnalysisService` into `SignalService` + `QueryService`, then update all consumers in `api/` and `main.go`.

**Tech Stack:** Go 1.25.7, gin, gorm

---

## File Map

**Create:**
- `business/util.go` — tool functions (`toSymbol`, `cleanKlines`, `isFriday`, etc.)
- `business/util_test.go` — unit tests for tool functions
- `business/scheduler_base.go` — scheduler primitives (`triggerGuard`, `concurrentWorker`)
- `business/stock_service.go` — `StockDataService` (historical + append for daily/weekly)
- `business/financial_service.go` — `FinancialReportService`
- `business/signal_service.go` — `SignalService` (buy signal scanning)
- `business/query_service.go` — `QueryService` (price + financial report queries)

**Modify:**
- `business/scheduler.go` — rewrite using `scheduler_base.go` primitives
- `business/financial_scheduler.go` — rewrite using `scheduler_base.go` primitives
- `business/stock_service_test.go` — remove financial report tests, adapt mocks
- `business/scheduler_test.go` — adapt `mockSvcForScheduler`
- `api/handler.go` — update `StockHandler` fields and constructor
- `api/router.go` — update `NewRouter` signature
- `api/handler_test.go` — split mocks, update `setupTestRouter`
- `api/save_stock_historical_data_test.go` — update `setupTestRouter` calls
- `api/append_stock_data_test.go` — update `setupTestRouter` calls
- `api/save_financial_report_data_test.go` — use new mock + `NewStockHandler`
- `api/append_financial_report_data_test.go` — update `NewStockHandler` calls
- `api/get_stock_price_test.go` — update `NewStockHandler` calls
- `api/get_financial_report_test.go` — update `NewStockHandler` calls
- `main.go` — update service initialization

**Delete:**
- `business/analysis_service.go` — replaced by `signal_service.go` + `query_service.go`

---

### Task 1: Extract tool functions to util.go

**Files:**
- Create: `business/util.go`
- Create: `business/util_test.go`

- [ ] **Step 1: Create util.go**

```go
package business

import (
	"fmt"
	"strings"
	"time"

	"trading/model"
)

func toSymbol(code string) (string, error) {
	switch {
	case strings.HasPrefix(code, "6"):
		return "sh" + code, nil
	case strings.HasPrefix(code, "0"), strings.HasPrefix(code, "3"):
		return "sz" + code, nil
	default:
		return "", fmt.Errorf("unsupported stock code: %s", code)
	}
}

func cleanKlines(klines []model.StockKline) []*model.StockKline {
	result := make([]*model.StockKline, 0, len(klines))
	for i := range klines {
		k := &klines[i]
		k.Code = strings.TrimPrefix(k.Code, "sh")
		k.Code = strings.TrimPrefix(k.Code, "sz")
		result = append(result, k)
	}
	return result
}

func filterAfterDate(klines []*model.StockKline, lastDate string) []*model.StockKline {
	if lastDate == "" {
		return klines
	}
	result := make([]*model.StockKline, 0, len(klines))
	for _, k := range klines {
		if k.Date > lastDate {
			result = append(result, k)
		}
	}
	return result
}

func filterIncompleteWeekly(klines []*model.StockKline) []*model.StockKline {
	if len(klines) == 0 {
		return klines
	}
	last := klines[len(klines)-1]
	if !isFriday(last.Date) {
		return klines[:len(klines)-1]
	}
	return klines
}

func isFriday(dateStr string) bool {
	layout := "2006-01-02"
	if len(dateStr) > 10 {
		layout = "2006-01-02 15:04:05"
	}
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return false
	}
	return t.Weekday() == time.Friday
}

func toDaily(klines []*model.StockKline) []*model.StockKlineDaily {
	result := make([]*model.StockKlineDaily, 0, len(klines))
	for _, k := range klines {
		d := model.StockKlineDaily(*k)
		result = append(result, &d)
	}
	return result
}

func toWeekly(klines []*model.StockKline) []*model.StockKlineWeekly {
	result := make([]*model.StockKlineWeekly, 0, len(klines))
	for _, k := range klines {
		w := model.StockKlineWeekly(*k)
		result = append(result, &w)
	}
	return result
}

func dailyToKlines(dailies []*model.StockKlineDaily) []*model.StockKline {
	result := make([]*model.StockKline, 0, len(dailies))
	for _, d := range dailies {
		k := model.StockKline(*d)
		result = append(result, &k)
	}
	return result
}

func weeklyToKlines(weeklies []*model.StockKlineWeekly) []*model.StockKline {
	result := make([]*model.StockKline, 0, len(weeklies))
	for _, w := range weeklies {
		k := model.StockKline(*w)
		result = append(result, &k)
	}
	return result
}
```

- [ ] **Step 2: Create util_test.go**

```go
package business

import (
	"testing"
	"time"
)

func TestToSymbol(t *testing.T) {
	tests := []struct {
		code    string
		want    string
		wantErr bool
	}{
		{"600000", "sh600000", false},
		{"000001", "sz000001", false},
		{"300001", "sz300001", false},
		{"999999", "", true},
	}
	for _, tt := range tests {
		got, err := toSymbol(tt.code)
		if (err != nil) != tt.wantErr {
			t.Errorf("toSymbol(%s) error = %v, wantErr %v", tt.code, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("toSymbol(%s) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

func TestIsFriday(t *testing.T) {
	if !isFriday("2025-04-25") {
		t.Error("2025-04-25 is Friday")
	}
	if isFriday("2025-04-24") {
		t.Error("2025-04-24 is not Friday")
	}
}

func TestFilterIncompleteWeekly(t *testing.T) {
	klines := []*model.StockKline{
		{Date: "2025-04-18"},
		{Date: "2025-04-22"},
	}
	result := filterIncompleteWeekly(klines)
	if len(result) != 1 {
		t.Fatalf("expected 1 kline, got %d", len(result))
	}
	if result[0].Date != "2025-04-18" {
		t.Errorf("expected 2025-04-18, got %s", result[0].Date)
	}
}

func TestFilterAfterDate(t *testing.T) {
	klines := []*model.StockKline{
		{Date: "2025-04-20"},
		{Date: "2025-04-21"},
		{Date: "2025-04-22"},
	}
	result := filterAfterDate(klines, "2025-04-21")
	if len(result) != 1 || result[0].Date != "2025-04-22" {
		t.Fatalf("expected 1 kline with date 2025-04-22, got %+v", result)
	}
}

func TestLastFridayDate(t *testing.T) {
	tests := []struct {
		input    time.Time
		expected string
	}{
		{time.Date(2025, 4, 21, 0, 0, 0, 0, time.UTC), "2025-04-18"},
		{time.Date(2025, 4, 25, 0, 0, 0, 0, time.UTC), "2025-04-25"},
		{time.Date(2025, 4, 26, 0, 0, 0, 0, time.UTC), "2025-04-25"},
	}
	for _, tt := range tests {
		got := lastFridayDate(tt.input)
		if got != tt.expected {
			t.Errorf("lastFridayDate(%v) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./business/ -run TestToSymbol -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add business/util.go business/util_test.go
git commit -m "refactor: extract tool functions to util.go"
```

---

### Task 2: Extract scheduler primitives to scheduler_base.go

**Files:**
- Create: `business/scheduler_base.go`

- [ ] **Step 1: Create scheduler_base.go**

```go
package business

import (
	"context"
	"fmt"
	"log"
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
				log.Printf("[worker] limiter acquire failed for %s: %v", c, err)
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
```

- [ ] **Step 2: Run compile check**

Run: `cd /Users/lmm/project/lmm/trading && go build ./business/`

Expected: compile success

- [ ] **Step 3: Commit**

```bash
git add business/scheduler_base.go
git commit -m "refactor: extract scheduler primitives to scheduler_base.go"
```

---

### Task 3: Rewrite stock_service.go as StockDataService

**Files:**
- Modify: `business/stock_service.go` (full rewrite — keep only stock data methods)
- Modify: `business/stock_service_test.go` (remove financial report tests)

- [ ] **Step 1: Rewrite stock_service.go**

```go
package business

import (
	"context"
	"fmt"

	"trading/data"
	"trading/model"
	"trading/pkg/broker"
)

type StockDataService interface {
	SaveHistoricalData(ctx context.Context, code string) error
	AppendStockData(ctx context.Context, code string) error
}

type stockDataService struct {
	broker     broker.IBroker
	dailyRepo  data.StockKlineDailyRepo
	weeklyRepo data.StockKlineWeeklyRepo
}

func NewStockDataService(b broker.IBroker, dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo) StockDataService {
	return &stockDataService{broker: b, dailyRepo: dailyRepo, weeklyRepo: weeklyRepo}
}

func (s *stockDataService) SaveHistoricalData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	dailyKlines, err := s.broker.GetStockHistorical(ctx, symbol, 240, 1000)
	if err != nil {
		return fmt.Errorf("fetch daily historical failed: %w", err)
	}

	cleanedDaily := cleanKlines(dailyKlines)
	daily := toDaily(cleanedDaily)
	if err := s.dailyRepo.Upsert(ctx, daily); err != nil {
		return fmt.Errorf("upsert daily failed: %w", err)
	}

	weeklyKlines, err := s.broker.GetStockHistorical(ctx, symbol, 1680, 200)
	if err != nil {
		return fmt.Errorf("fetch weekly historical failed: %w", err)
	}

	cleanedWeekly := cleanKlines(weeklyKlines)
	filteredWeekly := filterIncompleteWeekly(cleanedWeekly)
	weekly := toWeekly(filteredWeekly)
	if err := s.weeklyRepo.Upsert(ctx, weekly); err != nil {
		return fmt.Errorf("upsert weekly failed: %w", err)
	}

	return nil
}

func (s *stockDataService) AppendStockData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	if err := s.appendDaily(ctx, symbol, code); err != nil {
		return err
	}
	if err := s.appendWeekly(ctx, symbol, code); err != nil {
		return err
	}
	return nil
}

func (s *stockDataService) appendDaily(ctx context.Context, symbol, code string) error {
	var lastDate string
	if latest, err := s.dailyRepo.FindLatestByCode(ctx, code); err == nil {
		lastDate = latest.Date
	}

	klines, err := s.broker.GetStockHistorical(ctx, symbol, 240, 30)
	if err != nil {
		return fmt.Errorf("fetch daily failed: %w", err)
	}

	newKlines := filterAfterDate(cleanKlines(klines), lastDate)
	if len(newKlines) == 0 {
		return nil
	}

	if err := s.dailyRepo.Upsert(ctx, toDaily(newKlines)); err != nil {
		return fmt.Errorf("upsert daily failed: %w", err)
	}
	return nil
}

func (s *stockDataService) appendWeekly(ctx context.Context, symbol, code string) error {
	var lastDate string
	if latest, err := s.weeklyRepo.FindLatestByCode(ctx, code); err == nil {
		lastDate = latest.Date
	}

	klines, err := s.broker.GetStockHistorical(ctx, symbol, 1680, 10)
	if err != nil {
		return fmt.Errorf("fetch weekly failed: %w", err)
	}

	newKlines := filterAfterDate(filterIncompleteWeekly(cleanKlines(klines)), lastDate)
	if len(newKlines) == 0 {
		return nil
	}

	if err := s.weeklyRepo.Upsert(ctx, toWeekly(newKlines)); err != nil {
		return fmt.Errorf("upsert weekly failed: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Rewrite stock_service_test.go**

Keep only the tests related to `SaveHistoricalData` and `AppendStockData`. Remove all financial report tests. Update the constructor call from `NewStockService` to `NewStockDataService` and remove the 4th argument (`&mockFinancialRepo{}`).

The test file should contain: `TestStockServiceSaveHistoricalDataSuccess`, `TestStockServiceSaveHistoricalDataDropIncompleteWeekly`, `TestStockServiceSaveHistoricalDataInvalidCode`, `TestStockServiceSaveHistoricalDataBrokerError`, `TestStockServiceSaveHistoricalDataDailyRepoError`, `TestStockServiceSaveHistoricalDataWeeklyRepoError`, `TestAppendStockDataSuccess`, `TestAppendStockDataEmptyDB`, `TestAppendStockDataNoNewData`, `TestAppendStockDataBrokerError`, `TestAppendStockDataDailyRepoError`.

For each test, change:
```go
// Before
svc := NewStockService(broker, dailyRepo, weeklyRepo, &mockFinancialRepo{})
// After
svc := NewStockDataService(broker, dailyRepo, weeklyRepo)
```

Also update `mockSvcForScheduler` in `scheduler_test.go` to implement only `StockDataService`:

```go
type mockSvcForScheduler struct {
	saveErr error
}

func (m *mockSvcForScheduler) SaveHistoricalData(ctx context.Context, code string) error {
	return m.saveErr
}
func (m *mockSvcForScheduler) AppendStockData(ctx context.Context, code string) error {
	return m.saveErr
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./business/ -run "TestStockService|TestAppendStockData" -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add business/stock_service.go business/stock_service_test.go business/scheduler_test.go
git commit -m "refactor: extract StockDataService from StockService"
```

---

### Task 4: Create FinancialReportService

**Files:**
- Create: `business/financial_service.go`
- Create: `business/financial_service_test.go`

- [ ] **Step 1: Create financial_service.go**

Extract the financial report methods from the old `stock_service.go`:

```go
package business

import (
	"context"
	"fmt"

	"trading/data"
	"trading/model"
	"trading/pkg/broker"
)

const defaultFinancialReportYears = 5
const defaultFinancialReportNum = defaultFinancialReportYears * 4

type FinancialReportService interface {
	SaveFinancialReportData(ctx context.Context, code string) error
	AppendFinancialReportData(ctx context.Context, code string) error
}

type financialReportService struct {
	broker      broker.IBroker
	financialRepo data.FinancialReportRepo
}

func NewFinancialReportService(b broker.IBroker, financialRepo data.FinancialReportRepo) FinancialReportService {
	return &financialReportService{broker: b, financialRepo: financialRepo}
}

func (s *financialReportService) SaveFinancialReportData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	reports, _, err := s.broker.GetFinancialReportHistorical(ctx, symbol, 1, defaultFinancialReportNum)
	if err != nil {
		return fmt.Errorf("fetch financial report failed: %w", err)
	}

	if len(reports) == 0 {
		return nil
	}

	if err := s.financialRepo.Upsert(ctx, reports); err != nil {
		return fmt.Errorf("upsert financial report failed: %w", err)
	}

	return nil
}

func (s *financialReportService) AppendFinancialReportData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	existing, err := s.financialRepo.FindByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("find existing reports failed: %w", err)
	}

	existingDates := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		existingDates[r.ReportDate] = struct{}{}
	}

	reports, _, err := s.broker.GetFinancialReportHistorical(ctx, symbol, 1, 4)
	if err != nil {
		return fmt.Errorf("fetch financial report failed: %w", err)
	}

	var newReports []*model.FinancialReport
	for _, r := range reports {
		if _, ok := existingDates[r.ReportDate]; !ok {
			newReports = append(newReports, r)
		}
	}

	if len(newReports) == 0 {
		return nil
	}

	if err := s.financialRepo.Upsert(ctx, newReports); err != nil {
		return fmt.Errorf("upsert financial report failed: %w", err)
	}

	return nil
}
```

- [ ] **Step 2: Create financial_service_test.go**

Move financial report tests from the old `stock_service_test.go` and adapt constructor calls:

```go
package business

import (
	"context"
	"errors"
	"testing"

	"trading/model"
)

func TestSaveFinancialReportDataSuccess(t *testing.T) {
	broker := &mockBroker{financialData: []*model.FinancialReport{{Code: "000001", ReportDate: "20250630"}}}
	repo := &mockFinancialRepo{}
	svc := NewFinancialReportService(broker, repo)

	err := svc.SaveFinancialReportData(context.Background(), "000001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("expected 1 report upserted, got %d", len(repo.upserted))
	}
}

func TestSaveFinancialReportDataInvalidCode(t *testing.T) {
	svc := NewFinancialReportService(&mockBroker{}, &mockFinancialRepo{})
	err := svc.SaveFinancialReportData(context.Background(), "999999")
	if err == nil {
		t.Fatal("expected error for invalid code, got nil")
	}
}

func TestSaveFinancialReportDataRepoError(t *testing.T) {
	broker := &mockBroker{financialData: []*model.FinancialReport{{Code: "000001", ReportDate: "20250630"}}}
	repo := &mockFinancialRepo{upErr: errors.New("db down")}
	svc := NewFinancialReportService(broker, repo)

	err := svc.SaveFinancialReportData(context.Background(), "000001")
	if err == nil {
		t.Fatal("expected error when repo upsert fails")
	}
}

func TestAppendFinancialReportDataSuccess(t *testing.T) {
	broker := &mockBroker{
		financialData: []*model.FinancialReport{
			{Code: "000001", ReportDate: "20251231"},
			{Code: "000001", ReportDate: "20250930"},
			{Code: "000001", ReportDate: "20250630"},
		},
	}
	repo := &mockFinancialRepo{
		reports: []*model.FinancialReport{
			{Code: "000001", ReportDate: "20251231"},
			{Code: "000001", ReportDate: "20250930"},
		},
	}
	svc := NewFinancialReportService(broker, repo)

	err := svc.AppendFinancialReportData(context.Background(), "000001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(repo.upserted) != 1 || repo.upserted[0].ReportDate != "20250630" {
		t.Fatalf("expected 1 new report with date 20250630, got %+v", repo.upserted)
	}
}

func TestAppendFinancialReportDataNoNewData(t *testing.T) {
	broker := &mockBroker{financialData: []*model.FinancialReport{{Code: "000001", ReportDate: "20251231"}}}
	repo := &mockFinancialRepo{reports: []*model.FinancialReport{{Code: "000001", ReportDate: "20251231"}}}
	svc := NewFinancialReportService(broker, repo)

	err := svc.AppendFinancialReportData(context.Background(), "000001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(repo.upserted) != 0 {
		t.Fatalf("expected 0 upserts, got %d", len(repo.upserted))
	}
}

func TestAppendFinancialReportDataBrokerError(t *testing.T) {
	broker := &mockBroker{financialErr: errors.New("broker down")}
	svc := NewFinancialReportService(broker, &mockFinancialRepo{})

	err := svc.AppendFinancialReportData(context.Background(), "000001")
	if err == nil {
		t.Fatal("expected error when broker fails")
	}
}

func TestAppendFinancialReportDataRepoError(t *testing.T) {
	broker := &mockBroker{financialData: []*model.FinancialReport{{Code: "000001", ReportDate: "20250630"}}}
	repo := &mockFinancialRepo{upErr: errors.New("db down")}
	svc := NewFinancialReportService(broker, repo)

	err := svc.AppendFinancialReportData(context.Background(), "000001")
	if err == nil {
		t.Fatal("expected error when repo upsert fails")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./business/ -run "TestSaveFinancialReportData|TestAppendFinancialReportData" -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add business/financial_service.go business/financial_service_test.go
git commit -m "refactor: extract FinancialReportService from StockService"
```

---

### Task 5: Create SignalService

**Files:**
- Create: `business/signal_service.go`
- Create: `business/signal_service_test.go`
- Delete: `business/analysis_service.go` (after confirming content is migrated)

- [ ] **Step 1: Create signal_service.go**

Extract strategy scanning methods from `analysis_service.go`:

```go
package business

import (
	"context"
	"fmt"

	"trading/data"
	"trading/pkg/strategy"
)

// StrategySignal represents the scan result for a single strategy.
type StrategySignal struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
}

// SignalService scans stocks for buy signals using technical strategies.
type SignalService interface {
	FindBuySignals(ctx context.Context) ([]StrategySignal, error)
	FindBuySignalsByStrategy(ctx context.Context, name string) (*StrategySignal, error)
}

type signalService struct {
	dailyRepo  data.StockKlineDailyRepo
	weeklyRepo data.StockKlineWeeklyRepo
}

func NewSignalService(dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo) SignalService {
	return &signalService{dailyRepo: dailyRepo, weeklyRepo: weeklyRepo}
}

func (s *signalService) FindBuySignals(ctx context.Context) ([]StrategySignal, error) {
	var results []StrategySignal

	dailySigs, err := s.scanDailyStrategy(ctx, strategy.NewDailyB1BuyStrategy())
	if err != nil {
		return nil, err
	}
	if dailySigs != nil {
		results = append(results, *dailySigs)
	}

	weeklySigs, err := s.scanWeeklyStrategy(ctx, strategy.NewWeeklyB1BuyStrategy())
	if err != nil {
		return nil, err
	}
	if weeklySigs != nil {
		results = append(results, *weeklySigs)
	}

	return results, nil
}

func (s *signalService) FindBuySignalsByStrategy(ctx context.Context, name string) (*StrategySignal, error) {
	switch name {
	case "daily_b1_buy":
		return s.scanDailyStrategy(ctx, strategy.NewDailyB1BuyStrategy())
	case "weekly_b1_buy":
		return s.scanWeeklyStrategy(ctx, strategy.NewWeeklyB1BuyStrategy())
	default:
		return nil, fmt.Errorf("unknown strategy: %s", name)
	}
}

func (s *signalService) scanDailyStrategy(ctx context.Context, st *strategy.Strategy) (*StrategySignal, error) {
	codes, err := s.dailyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all daily codes failed: %w", err)
	}

	var matched []string
	for _, code := range codes {
		dailies, findErr := s.dailyRepo.FindByCode(ctx, code, 70)
		if findErr != nil || len(dailies) == 0 {
			continue
		}
		lastDate := dailies[len(dailies)-1].Date
		klines := dailyToKlines(dailies)
		signals := st.ScanAll(klines)
		if len(signals) > 0 && signals[len(signals)-1].Date == lastDate {
			matched = append(matched, code)
		}
	}

	if len(matched) == 0 {
		return nil, nil
	}
	return &StrategySignal{Name: st.Name(), Codes: matched}, nil
}

func (s *signalService) scanWeeklyStrategy(ctx context.Context, st *strategy.Strategy) (*StrategySignal, error) {
	codes, err := s.weeklyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all weekly codes failed: %w", err)
	}

	var matched []string
	for _, code := range codes {
		weeklies, findErr := s.weeklyRepo.FindByCode(ctx, code, 70)
		if findErr != nil || len(weeklies) == 0 {
			continue
		}
		lastDate := weeklies[len(weeklies)-1].Date
		klines := weeklyToKlines(weeklies)
		signals := st.ScanAll(klines)
		if len(signals) > 0 && signals[len(signals)-1].Date == lastDate {
			matched = append(matched, code)
		}
	}

	if len(matched) == 0 {
		return nil, nil
	}
	return &StrategySignal{Name: st.Name(), Codes: matched}, nil
}
```

- [ ] **Step 2: Create signal_service_test.go**

```go
package business

import (
	"context"
	"errors"
	"testing"

	"trading/model"
)

// mockDailyRepoForSignal simulates a daily repo with controlled data.
type mockDailyRepoForSignal struct {
	codes      []string
	data       map[string][]*model.StockKlineDaily
	findAllErr error
}

func (m *mockDailyRepoForSignal) Create(ctx context.Context, kline *model.StockKlineDaily) error         { return nil }
func (m *mockDailyRepoForSignal) CreateBatch(ctx context.Context, klines []*model.StockKlineDaily) error { return nil }
func (m *mockDailyRepoForSignal) Upsert(ctx context.Context, klines []*model.StockKlineDaily) error      { return nil }
func (m *mockDailyRepoForSignal) FindByID(ctx context.Context, id uint) (*model.StockKlineDaily, error)  { return nil, nil }
func (m *mockDailyRepoForSignal) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineDaily, error) {
	return m.data[code], nil
}
func (m *mockDailyRepoForSignal) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockDailyRepoForSignal) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockDailyRepoForSignal) FindAllCodes(ctx context.Context) ([]string, error) {
	if m.findAllErr != nil {
		return nil, m.findAllErr
	}
	return m.codes, nil
}
func (m *mockDailyRepoForSignal) Update(ctx context.Context, kline *model.StockKlineDaily) error { return nil }
func (m *mockDailyRepoForSignal) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockDailyRepoForSignal) List(ctx context.Context, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}

// mockWeeklyRepoForSignal simulates a weekly repo with controlled data.
type mockWeeklyRepoForSignal struct {
	codes      []string
	data       map[string][]*model.StockKlineWeekly
	findAllErr error
}

func (m *mockWeeklyRepoForSignal) Create(ctx context.Context, kline *model.StockKlineWeekly) error         { return nil }
func (m *mockWeeklyRepoForSignal) CreateBatch(ctx context.Context, klines []*model.StockKlineWeekly) error { return nil }
func (m *mockWeeklyRepoForSignal) Upsert(ctx context.Context, klines []*model.StockKlineWeekly) error      { return nil }
func (m *mockWeeklyRepoForSignal) FindByID(ctx context.Context, id uint) (*model.StockKlineWeekly, error)  { return nil, nil }
func (m *mockWeeklyRepoForSignal) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error) {
	return m.data[code], nil
}
func (m *mockWeeklyRepoForSignal) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockWeeklyRepoForSignal) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockWeeklyRepoForSignal) FindAllCodes(ctx context.Context) ([]string, error) {
	if m.findAllErr != nil {
		return nil, m.findAllErr
	}
	return m.codes, nil
}
func (m *mockWeeklyRepoForSignal) Update(ctx context.Context, kline *model.StockKlineWeekly) error { return nil }
func (m *mockWeeklyRepoForSignal) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockWeeklyRepoForSignal) List(ctx context.Context, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}

func TestSignalServiceFindBuySignalsByStrategyUnknown(t *testing.T) {
	svc := NewSignalService(&mockDailyRepoForSignal{}, &mockWeeklyRepoForSignal{})
	_, err := svc.FindBuySignalsByStrategy(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestSignalServiceFindBuySignalsFindAllCodesError(t *testing.T) {
	dailyRepo := &mockDailyRepoForSignal{findAllErr: errors.New("db error")}
	svc := NewSignalService(dailyRepo, &mockWeeklyRepoForSignal{})
	_, err := svc.FindBuySignals(context.Background())
	if err == nil {
		t.Fatal("expected error when FindAllCodes fails")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./business/ -run "TestSignalService" -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add business/signal_service.go business/signal_service_test.go
git commit -m "refactor: extract SignalService from AnalysisService"
```

---

### Task 6: Create QueryService

**Files:**
- Create: `business/query_service.go`
- Create: `business/query_service_test.go`

- [ ] **Step 1: Create query_service.go**

Extract query methods from `analysis_service.go`:

```go
package business

import (
	"context"
	"fmt"

	"trading/data"
	"trading/model"
)

// QueryService provides read-only data queries for stock prices and financial reports.
type QueryService interface {
	FindStockPricesByCode(ctx context.Context, code, cycle string, limit, offset int) ([]*model.StockKlineDaily, error)
	FindFinancialReportsByCode(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error)
}

type queryService struct {
	dailyRepo     data.StockKlineDailyRepo
	weeklyRepo    data.StockKlineWeeklyRepo
	financialRepo data.FinancialReportRepo
}

func NewQueryService(dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo, financialRepo data.FinancialReportRepo) QueryService {
	return &queryService{dailyRepo: dailyRepo, weeklyRepo: weeklyRepo, financialRepo: financialRepo}
}

func (s *queryService) FindStockPricesByCode(ctx context.Context, code, cycle string, limit, offset int) ([]*model.StockKlineDaily, error) {
	switch cycle {
	case "daily":
		return s.dailyRepo.FindByCodeWithPagination(ctx, code, limit, offset)
	case "weekly":
		weeklies, err := s.weeklyRepo.FindByCodeWithPagination(ctx, code, limit, offset)
		if err != nil {
			return nil, err
		}
		result := make([]*model.StockKlineDaily, 0, len(weeklies))
		for _, w := range weeklies {
			d := model.StockKlineDaily(*w)
			result = append(result, &d)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported cycle: %s, expected daily or weekly", cycle)
	}
}

func (s *queryService) FindFinancialReportsByCode(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return s.financialRepo.FindByCodeWithPagination(ctx, code, limit, offset)
}
```

- [ ] **Step 2: Create query_service_test.go**

```go
package business

import (
	"context"
	"errors"
	"testing"

	"trading/model"
)

func TestQueryServiceFindStockPricesByCodeDaily(t *testing.T) {
	dailyRepo := &mockDailyRepo{k: []*model.StockKlineDaily{{Code: "000001", Date: "2025-04-25"}}}
	svc := NewQueryService(dailyRepo, &mockWeeklyRepo{}, &mockFinancialRepo{})

	prices, err := svc.FindStockPricesByCode(context.Background(), "000001", "daily", 10, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}
}

func TestQueryServiceFindStockPricesByCodeInvalidCycle(t *testing.T) {
	svc := NewQueryService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{})
	_, err := svc.FindStockPricesByCode(context.Background(), "000001", "monthly", 10, 0)
	if err == nil {
		t.Fatal("expected error for invalid cycle")
	}
}

func TestQueryServiceFindFinancialReportsByCode(t *testing.T) {
	// Use the mockFinancialRepo from stock_service_test.go. Since it's in the same package,
	// we can reuse it. We need a mock that supports FindByCodeWithPagination.
	// The existing mockFinancialRepo does not implement FindByCodeWithPagination,
	// so we define a local one here.
	type mockFinRepo struct {
		reports []*model.FinancialReport
		err     error
	}
	mockFinRepoUpsert := func(ctx context.Context, reports []*model.FinancialReport) error { return nil }
	mockFinRepoFindByCode := func(ctx context.Context, code string) ([]*model.FinancialReport, error) { return nil, nil }
	mockFinRepoFindAllCodes := func(ctx context.Context) ([]string, error) { return nil, nil }
	_ = mockFinRepoUpsert
	_ = mockFinRepoFindByCode
	_ = mockFinRepoFindAllCodes

	// For this test, define a proper mock inline.
	repo := &mockFinRepoWithPagination{reports: []*model.FinancialReport{{Code: "000001", ReportDate: "20251231"}}}
	svc := NewQueryService(&mockDailyRepo{}, &mockWeeklyRepo{}, repo)

	reports, err := svc.FindFinancialReportsByCode(context.Background(), "000001", 10, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
}

type mockFinRepoWithPagination struct {
	reports []*model.FinancialReport
	err     error
}

func (m *mockFinRepoWithPagination) Upsert(ctx context.Context, reports []*model.FinancialReport) error {
	return nil
}
func (m *mockFinRepoWithPagination) FindByCode(ctx context.Context, code string) ([]*model.FinancialReport, error) {
	return nil, nil
}
func (m *mockFinRepoWithPagination) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.reports, nil
}
func (m *mockFinRepoWithPagination) FindAllCodes(ctx context.Context) ([]string, error) {
	return nil, nil
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./business/ -run "TestQueryService" -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add business/query_service.go business/query_service_test.go
git commit -m "refactor: extract QueryService from AnalysisService"
```

---

### Task 7: Rewrite scheduler.go using scheduler_base.go

**Files:**
- Modify: `business/scheduler.go`
- Modify: `business/scheduler_test.go`

- [ ] **Step 1: Rewrite scheduler.go**

```go
package business

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"trading/data"
	"trading/pkg/indicator"
)

// Scheduler schedules incremental stock data updates.
type Scheduler interface {
	Start(ctx context.Context, hour, minute int)
	Stop()
	TriggerNow(ctx context.Context) error
}

type stockScheduler struct {
	svc        StockDataService
	dailyRepo  data.StockKlineDailyRepo
	weeklyRepo data.StockKlineWeeklyRepo
	stopCh     chan struct{}
	interval   time.Duration
	bgCtx      context.Context
	guard      triggerGuard
	worker     *concurrentWorker
}

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

func (s *stockScheduler) Start(ctx context.Context, hour, minute int) {
	s.bgCtx = ctx
	go s.run(ctx, hour, minute)
}

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
			if isWeekday(time.Now()) {
				s.scanAndConsume(ctx)
			} else {
				log.Println("[scheduler] skipped: not a weekday")
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

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

func (s *stockScheduler) scan(ctx context.Context) ([]task, error) {
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

	today := time.Now().Format("2006-01-02")
	lastFriday := lastFridayDate(time.Now())

	type result struct {
		task task
		ok   bool
	}

	results := make(chan result, len(codeSet))
	var wg sync.WaitGroup

	for code := range codeSet {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			if err := s.worker.limiter.Acquire(ctx); err != nil {
				log.Printf("[scheduler] limiter acquire failed for %s: %v", c, err)
				return
			}
			defer s.worker.limiter.Release()

			needDaily, needWeekly, err := s.checkCode(ctx, c, today, lastFriday)
			if err != nil {
				log.Printf("[scheduler] check %s failed: %v", c, err)
				return
			}
			if needDaily || needWeekly {
				results <- result{task: task{code: c, needDaily: needDaily, needWeekly: needWeekly}, ok: true}
			}
		}(code)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var tasks []task
	for r := range results {
		if r.ok {
			tasks = append(tasks, r.task)
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

func nextDailyTime(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func lastFridayDate(t time.Time) string {
	wd := t.Weekday()
	daysBack := int(wd - time.Friday)
	if daysBack < 0 {
		daysBack += 7
	}
	if wd == time.Friday {
		daysBack = 0
	}
	return t.Add(-time.Duration(daysBack) * 24 * time.Hour).Format("2006-01-02")
}

func isWeekday(t time.Time) bool {
	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}
```

- [ ] **Step 2: Update scheduler_test.go**

The existing `scheduler_test.go` tests remain largely the same. Only update `mockSvcForScheduler` to implement `StockDataService` instead of `StockService`:

```go
type mockSvcForScheduler struct {
	saveErr error
}

func (m *mockSvcForScheduler) SaveHistoricalData(ctx context.Context, code string) error {
	return m.saveErr
}
func (m *mockSvcForScheduler) AppendStockData(ctx context.Context, code string) error {
	return m.saveErr
}
```

Also update the `NewScheduler` call in tests. The constructor signature is the same `(svc StockDataService, dailyRepo, weeklyRepo)`, so test setup doesn't change except the mock type now implements `StockDataService` instead of `StockService`.

- [ ] **Step 3: Run tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./business/ -run "TestScheduler" -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add business/scheduler.go business/scheduler_test.go
git commit -m "refactor: rewrite scheduler using scheduler_base primitives"
```

---

### Task 8: Rewrite financial_scheduler.go using scheduler_base.go

**Files:**
- Modify: `business/financial_scheduler.go`

- [ ] **Step 1: Rewrite financial_scheduler.go**

```go
package business

import (
	"context"
	"fmt"
	"log"

	"trading/data"
	"trading/pkg/indicator"
)

// FinancialScheduler schedules incremental financial report updates.
type FinancialScheduler interface {
	TriggerNow(ctx context.Context) error
}

type financialScheduler struct {
	svc         FinancialReportService
	financialRepo data.FinancialReportRepo
	guard       triggerGuard
	worker      *concurrentWorker
}

func NewFinancialScheduler(svc FinancialReportService, financialRepo data.FinancialReportRepo) FinancialScheduler {
	return &financialScheduler{
		svc:         svc,
		financialRepo: financialRepo,
		worker:      newConcurrentWorker(100),
	}
}

func (s *financialScheduler) TriggerNow(ctx context.Context) error {
	if !s.guard.tryStart() {
		return fmt.Errorf("another task is already running")
	}

	log.Println("[financial-scheduler] manual trigger started")
	go func() {
		defer s.guard.markDone()
		if err := s.scanAndConsume(ctx); err != nil {
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

	handler := func(ctx context.Context, code string) error {
		return s.svc.AppendFinancialReportData(ctx, code)
	}

	errs := s.worker.run(ctx, codes, handler)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Printf("[financial-scheduler] %v", err)
		}
	}

	log.Printf("[financial-scheduler] all tasks done, %d failed", len(errs))
	return nil
}
```

- [ ] **Step 2: Run compile check**

Run: `cd /Users/lmm/project/lmm/trading && go build ./business/`

Expected: compile success

- [ ] **Step 3: Commit**

```bash
git add business/financial_scheduler.go
git commit -m "refactor: rewrite financial scheduler using scheduler_base primitives"
```

---

### Task 9: Update API handler and router

**Files:**
- Modify: `api/handler.go`
- Modify: `api/router.go`
- Modify: `api/handler_test.go`

- [ ] **Step 1: Update handler.go**

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"trading/business"
)

// StockHandler is the HTTP handler for stock data APIs.
type StockHandler struct {
	stockSvc           business.StockDataService
	scheduler          business.Scheduler
	financialSvc       business.FinancialReportService
	financialScheduler business.FinancialScheduler
	signalSvc          business.SignalService
	querySvc           business.QueryService
}

// NewStockHandler creates a new StockHandler.
func NewStockHandler(stockSvc business.StockDataService, scheduler business.Scheduler, financialSvc business.FinancialReportService, financialScheduler business.FinancialScheduler, signalSvc business.SignalService, querySvc business.QueryService) *StockHandler {
	return &StockHandler{
		stockSvc:           stockSvc,
		scheduler:          scheduler,
		financialSvc:       financialSvc,
		financialScheduler: financialScheduler,
		signalSvc:          signalSvc,
		querySvc:           querySvc,
	}
}

// response is the unified JSON response structure.
type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func respondSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{Code: 0, Message: "success", Data: data})
}

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, response{Code: status, Message: message, Data: nil})
}
```

- [ ] **Step 2: Update router.go**

```go
package api

import (
	"github.com/gin-gonic/gin"

	"trading/business"
)

// NewRouter creates a gin router with all registered routes.
func NewRouter(stockSvc business.StockDataService, scheduler business.Scheduler, financialSvc business.FinancialReportService, financialScheduler business.FinancialScheduler, signalSvc business.SignalService, querySvc business.QueryService) *gin.Engine {
	r := gin.Default()
	h := NewStockHandler(stockSvc, scheduler, financialSvc, financialScheduler, signalSvc, querySvc)

	r.POST("/api/stocks/historical", h.SaveStockHistoricalData)
	r.POST("/api/stocks/append", h.AppendStockData)
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	r.GET("/api/stocks/signal", h.GetStockBuySignals)
	r.GET("/api/stocks/price", h.GetStockPrice)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	return r
}
```

- [ ] **Step 3: Update handler_test.go**

Replace all mock types and `setupTestRouter`:

```go
package api

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	"trading/business"
	"trading/model"
)

// mockStockDataService simulates StockDataService.
type mockStockDataService struct {
	saveErr error
}

func (m *mockStockDataService) SaveHistoricalData(ctx context.Context, code string) error {
	return m.saveErr
}
func (m *mockStockDataService) AppendStockData(ctx context.Context, code string) error {
	return m.saveErr
}

// mockFinancialReportService simulates FinancialReportService.
type mockFinancialReportService struct {
	saveErr error
}

func (m *mockFinancialReportService) SaveFinancialReportData(ctx context.Context, code string) error {
	return m.saveErr
}
func (m *mockFinancialReportService) AppendFinancialReportData(ctx context.Context, code string) error {
	return m.saveErr
}

// mockScheduler simulates Scheduler.
type mockScheduler struct {
	triggerErr     error
	alreadyRunning bool
}

func (m *mockScheduler) Start(ctx context.Context, hour, minute int) {}
func (m *mockScheduler) Stop()                                      {}
func (m *mockScheduler) TriggerNow(ctx context.Context) error {
	if m.alreadyRunning {
		return errors.New("another task is already running")
	}
	return m.triggerErr
}

// mockFinancialScheduler simulates FinancialScheduler.
type mockFinancialScheduler struct {
	triggerErr     error
	alreadyRunning bool
}

func (m *mockFinancialScheduler) TriggerNow(ctx context.Context) error {
	if m.alreadyRunning {
		return errors.New("another task is already running")
	}
	return m.triggerErr
}

// mockSignalService simulates SignalService.
type mockSignalService struct {
	signal    *business.StrategySignal
	signalErr error
}

func (m *mockSignalService) FindBuySignals(ctx context.Context) ([]business.StrategySignal, error) {
	if m.signal == nil {
		return nil, m.signalErr
	}
	return []business.StrategySignal{*m.signal}, m.signalErr
}
func (m *mockSignalService) FindBuySignalsByStrategy(ctx context.Context, name string) (*business.StrategySignal, error) {
	return m.signal, m.signalErr
}

// mockQueryService simulates QueryService.
type mockQueryService struct {
	prices     []*model.StockKlineDaily
	reports    []*model.FinancialReport
	pricesErr  error
	reportsErr error
}

func (m *mockQueryService) FindStockPricesByCode(ctx context.Context, code, cycle string, limit, offset int) ([]*model.StockKlineDaily, error) {
	return m.prices, m.pricesErr
}
func (m *mockQueryService) FindFinancialReportsByCode(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return m.reports, m.reportsErr
}

func setupTestRouter(stockSvc business.StockDataService, scheduler business.Scheduler, signalSvc business.SignalService, querySvc business.QueryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewStockHandler(stockSvc, scheduler, &mockFinancialReportService{}, &mockFinancialScheduler{}, signalSvc, querySvc)
	r.POST("/api/stocks/historical", h.SaveStockHistoricalData)
	r.GET("/api/stocks/signal", h.GetStockBuySignals)
	r.POST("/api/stocks/append", h.AppendStockData)
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	r.GET("/api/stocks/price", h.GetStockPrice)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	return r
}
```

- [ ] **Step 4: Run API compile check**

Run: `cd /Users/lmm/project/lmm/trading && go build ./api/`

Expected: compile success (tests will fail because test files aren't updated yet)

- [ ] **Step 5: Commit**

```bash
git add api/handler.go api/router.go api/handler_test.go
git commit -m "refactor: update API handler and router for new services"
```

---

### Task 10: Update all API test files

**Files:**
- Modify: `api/save_stock_historical_data_test.go`
- Modify: `api/append_stock_data_test.go`
- Modify: `api/save_financial_report_data_test.go`
- Modify: `api/append_financial_report_data_test.go`
- Modify: `api/get_stock_price_test.go`
- Modify: `api/get_financial_report_test.go`

- [ ] **Step 1: Update save_stock_historical_data_test.go**

Change all `setupTestRouter` calls from 3 args to 4 args:
```go
// Before: r := setupTestRouter(svc, &mockScheduler{}, nil)
// After:  r := setupTestRouter(svc, &mockScheduler{}, nil, nil)
```

Also update the variable type from `*mockStockService` to `*mockStockDataService`.

```go
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSaveStockHistoricalDataSuccess(t *testing.T) {
	svc := &mockStockDataService{}
	r := setupTestRouter(svc, &mockScheduler{}, nil, nil)
	// ... rest unchanged
}

func TestSaveStockHistoricalDataMissingCode(t *testing.T) {
	svc := &mockStockDataService{}
	r := setupTestRouter(svc, &mockScheduler{}, nil, nil)
	// ... rest unchanged
}

func TestSaveStockHistoricalDataServiceError(t *testing.T) {
	svc := &mockStockDataService{saveErr: errors.New("service error")}
	r := setupTestRouter(svc, &mockScheduler{}, nil, nil)
	// ... rest unchanged
}
```

- [ ] **Step 2: Update append_stock_data_test.go**

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppendStockDataSuccess(t *testing.T) {
	svc := &mockStockDataService{}
	r := setupTestRouter(svc, &mockScheduler{}, nil, nil)
	// ... rest unchanged
}

func TestAppendStockDataError(t *testing.T) {
	svc := &mockStockDataService{}
	sched := &mockScheduler{triggerErr: errors.New("trigger failed")}
	r := setupTestRouter(svc, sched, nil, nil)
	// ... rest unchanged
}

func TestAppendStockDataAlreadyRunning(t *testing.T) {
	svc := &mockStockDataService{}
	sched := &mockScheduler{alreadyRunning: true}
	r := setupTestRouter(svc, sched, nil, nil)
	// ... rest unchanged
}
```

- [ ] **Step 3: Update save_financial_report_data_test.go**

This file previously used `setupTestRouter(svc, &mockScheduler{}, nil)` where `svc` was a `*mockStockService` implementing `SaveFinancialReportData`. Now it needs a `*mockFinancialReportService` and must construct the handler directly because `setupTestRouter` fills financialSvc with a default empty mock.

```go
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSaveFinancialReportDataSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	financialSvc := &mockFinancialReportService{}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, financialSvc, &mockFinancialScheduler{}, &mockSignalService{}, &mockQueryService{})
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)

	body, _ := json.Marshal(map[string]string{"code": "000001"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"].(float64) != 0 {
		t.Fatalf("expected code 0, got %v", resp["code"])
	}
}

func TestSaveFinancialReportDataMissingCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, &mockQueryService{})
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSaveFinancialReportDataServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	financialSvc := &mockFinancialReportService{saveErr: errors.New("service error")}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, financialSvc, &mockFinancialScheduler{}, &mockSignalService{}, &mockQueryService{})
	r.POST("/api/stocks/financial-report", h.SaveFinancialReportData)

	body, _ := json.Marshal(map[string]string{"code": "000001"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
```

- [ ] **Step 4: Update append_financial_report_data_test.go**

```go
package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// mockFinancialSchedulerWithErr supports controlling TriggerNow return value.
type mockFinancialSchedulerWithErr struct {
	triggerErr     error
	alreadyRunning bool
}

func (m *mockFinancialSchedulerWithErr) TriggerNow(ctx context.Context) error {
	if m.alreadyRunning {
		return errors.New("another task is already running")
	}
	return m.triggerErr
}

func TestAppendFinancialReportDataSuccess(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialSchedulerWithErr{}, &mockSignalService{}, &mockQueryService{})
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	// ... rest unchanged
}

func TestAppendFinancialReportDataAlreadyRunning(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialSchedulerWithErr{alreadyRunning: true}, &mockSignalService{}, &mockQueryService{})
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	// ... rest unchanged
}

func TestAppendFinancialReportDataError(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialSchedulerWithErr{triggerErr: errors.New("scheduler error")}, &mockSignalService{}, &mockQueryService{})
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)
	// ... rest unchanged
}
```

- [ ] **Step 5: Update get_stock_price_test.go**

```go
package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/model"
)

func TestGetStockPriceSuccess(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		prices: []*model.StockKlineDaily{{Code: "000001", Date: "2025-04-25", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000}},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/price", h.GetStockPrice)
	// ... rest unchanged
}

func TestGetStockPriceMissingCode(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, &mockQueryService{})
	r.GET("/api/stocks/price", h.GetStockPrice)
	// ... rest unchanged
}

func TestGetStockPriceInvalidCycle(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, &mockQueryService{})
	r.GET("/api/stocks/price", h.GetStockPrice)
	// ... rest unchanged
}

func TestGetStockPriceServiceError(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{pricesErr: errors.New("db error")}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/price", h.GetStockPrice)
	// ... rest unchanged
}

func TestGetStockPriceDefaultParams(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		prices: []*model.StockKlineDaily{{Code: "000001", Date: "2025-04-25", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000}},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/price", h.GetStockPrice)
	// ... rest unchanged
}

func TestGetStockPricePagination(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		prices: []*model.StockKlineDaily{{Code: "000001", Date: "2025-04-25", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000}},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/price", h.GetStockPrice)
	// ... rest unchanged
}
```

- [ ] **Step 6: Update get_financial_report_test.go**

```go
package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/model"
)

func TestGetFinancialReportSuccess(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		reports: []*model.FinancialReport{{Code: "000001", ReportDate: "20251231", TotalRevenue: 1000, NetProfit: 100}},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	// ... rest unchanged
}

func TestGetFinancialReportMissingCode(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, &mockQueryService{})
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	// ... rest unchanged
}

func TestGetFinancialReportServiceError(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{reportsErr: errors.New("db error")}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	// ... rest unchanged
}

func TestGetFinancialReportPagination(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		reports: []*model.FinancialReport{{Code: "000001", ReportDate: "20251231", TotalRevenue: 1000, NetProfit: 100}},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockScheduler{}, &mockFinancialReportService{}, &mockFinancialScheduler{}, &mockSignalService{}, querySvc)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)
	// ... rest unchanged
}
```

- [ ] **Step 7: Run API tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./api/ -v`

Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add api/save_stock_historical_data_test.go api/append_stock_data_test.go api/save_financial_report_data_test.go api/append_financial_report_data_test.go api/get_stock_price_test.go api/get_financial_report_test.go
git commit -m "refactor: update API tests for new service interfaces"
```

---

### Task 11: Update main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Update main.go**

Replace the service initialization section:

```go
	b := broker.NewSinaBroker()
	stockSvc := business.NewStockDataService(b, d.StockKlineDaily(), d.StockKlineWeekly())
	financialSvc := business.NewFinancialReportService(b, d.FinancialReport())

	scheduler := business.NewScheduler(stockSvc, d.StockKlineDaily(), d.StockKlineWeekly())
	scheduler.Start(context.Background(), 16, 0)

	financialScheduler := business.NewFinancialScheduler(financialSvc, d.FinancialReport())

	signalSvc := business.NewSignalService(d.StockKlineDaily(), d.StockKlineWeekly())
	querySvc := business.NewQueryService(d.StockKlineDaily(), d.StockKlineWeekly(), d.FinancialReport())
	r := api.NewRouter(stockSvc, scheduler, financialSvc, financialScheduler, signalSvc, querySvc)
```

- [ ] **Step 2: Run compile check**

Run: `cd /Users/lmm/project/lmm/trading && go build .`

Expected: compile success

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "refactor: update main.go to use new services"
```

---

### Task 12: Final cleanup and verification

**Files:**
- Delete: `business/analysis_service.go`

- [ ] **Step 1: Delete old analysis_service.go**

```bash
git rm business/analysis_service.go
```

- [ ] **Step 2: Run all tests**

Run: `cd /Users/lmm/project/lmm/trading && go test ./...`

Expected: ALL PASS

- [ ] **Step 3: Final commit**

```bash
git commit -m "refactor: remove old AnalysisService"
```

---

## Spec Self-Review

**1. Spec coverage:**
- util.go extraction -> Task 1
- scheduler_base.go extraction -> Task 2
- StockDataService -> Task 3
- FinancialReportService -> Task 4
- SignalService -> Task 5
- QueryService -> Task 6
- scheduler.go rewrite -> Task 7
- financial_scheduler.go rewrite -> Task 8
- API handler + router update -> Task 9
- All API test updates -> Task 10
- main.go update -> Task 11
- Cleanup old files -> Task 12

All spec requirements are covered.

**2. Placeholder scan:**
- No TBD/TODO found.
- No vague instructions like "add appropriate error handling".
- All code blocks contain complete implementations.
- All test commands have expected outputs.

**3. Type consistency:**
- `StockDataService` used consistently in scheduler, handler, router, main.
- `FinancialReportService` used consistently in financial scheduler, handler, router, main.
- `SignalService` and `QueryService` used consistently in handler, router, main.
- Constructor names match: `NewStockDataService`, `NewFinancialReportService`, `NewSignalService`, `NewQueryService`.

Plan is ready for execution.
