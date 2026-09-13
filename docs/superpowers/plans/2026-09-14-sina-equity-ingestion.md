# Sina Equity Daily Ingestion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the production Eastmoney stock-history path with paced Sina raw daily bars plus qfq factors while preserving versioned Raw/ForwardAdjusted datasets and deriving weekly bars locally.

**Architecture:** `pkg/broker.SinaMarketSource` implements a stock-specific daily source port and owns the two fixed Sina HTTP protocols. `MarketIngestionService` merges daily history, derives weekly bars deterministically, validates factor coverage, and publishes one existing `MarketWriteBatch`; `main.go` injects one process-wide five-second token bucket shared by all Sina market requests.

**Tech Stack:** Go 1.25.7, `net/http`, `math/big`, `golang.org/x/time/rate`, existing market/application/port packages, GORM/MySQL 5.7 and 8.0, `httptest`, Testify.

**Spec:** `docs/superpowers/specs/2026-09-13-market-data-ingestion-design.md`

## Global Constraints

- Production stock history must not call `push2his.eastmoney.com` or automatically fall back to Eastmoney.
- Every Sina stock-history or qfq-factor HTTP attempt, including retries, consumes one token from `rate.NewLimiter(rate.Every(5*time.Second), 1)`.
- Upstream stock data consists only of raw daily bars and qfq factors; weekly bars are derived in Go from the same daily version.
- Existing `PENDING -> COMPLETE`, immutable-version reads, Raw and ForwardAdjusted semantics remain intact.
- Timestamps stay UTC `time.Time`/`DATETIME(6)`; prices remain signed integers scaled by 10000.
- Do not restore the old `business.StockDataService`, scheduler, or legacy K-line tables.
- Use injected `slog`/Telemetry and stable error codes; never log response bodies, cookies, credentials, or full request URLs.
- Overall test coverage must remain above 80%; changed market/application code must remain above 90%.
- Preserve the user's staged `.claude/skills/stock` deletions and never include them in feature commits.

---

### Task 1: Define the Daily Equity Source and Parse Sina Payloads

**Files:**
- Modify: `internal/port/market_source.go`
- Create: `pkg/broker/sina_market.go`
- Create: `pkg/broker/sina_market_test.go`

**Interfaces:**
- Consumes: `market.InstrumentID`, `market.Bar`, `market.AdjustmentFactor`, broker upstream error types.
- Produces:

```go
type EquityDailySource interface {
    FetchDailyBars(context.Context, market.InstrumentID, time.Time, time.Time) ([]market.Bar, error)
    FetchAdjustmentFactors(context.Context, market.InstrumentID) ([]market.AdjustmentFactor, error)
}

func NewSinaMarketSource(limiter *rate.Limiter) *SinaMarketSource
func NewSinaMarketSourceWithClient(client *http.Client, limiter *rate.Limiter, dailyBaseURL, factorBaseURL string) *SinaMarketSource
func ParseSinaDaily(body []byte, id market.InstrumentID, from, to time.Time) ([]market.Bar, error)
func ParseSinaQFQ(body []byte) ([]market.AdjustmentFactor, error)
```

- [ ] **Step 1: Write failing daily parser tests**

Add literal fixtures covering ordered bars, duplicate dates, zero/invalid OHLC, malformed JSON, empty arrays, and range filtering. Assert exact fixed-point values, UTC close dates, volume, `market.Day`, and version zero. Name the mutation each test catches, for example `TestParseSinaDailyRejectsDuplicateTradeDate`.

- [ ] **Step 2: Run the parser tests and verify RED**

Run: `go test ./pkg/broker -run 'TestParseSina(Daily|QFQ)' -count=1`

Expected: FAIL because the parser functions do not exist.

- [ ] **Step 3: Write failing qfq-factor tests**

Use the literal payload:

```text
var sh600000qfq={"total":2,"data":[{"d":"2026-07-16","f":"1.0000000000000000"},{"d":"2025-07-16","f":"1.2500000000000000"}]}
```

Assert ascending effective dates and factors `1/1` and `4/5`, because adjusted price is `raw/f`. Add malformed prefix, total mismatch, duplicate date, zero/negative factor, non-decimal factor, and int64 overflow cases.

- [ ] **Step 4: Implement strict parsers**

Implement JSON decoding with bounded response size. Parse prices through decimal strings into the existing 10000 scale. Parse qfq values with `big.Rat`, invert them, reduce the fraction, require positive int64 numerator/denominator, and never generate factors through `float64`.

```go
factor := new(big.Rat)
if _, ok := factor.SetString(row.Factor); !ok || factor.Sign() <= 0 {
    return nil, fmt.Errorf("%w: invalid qfq factor", ErrMalformedResponse)
}
factor.Inv(factor)
if !factor.Num().IsInt64() || !factor.Denom().IsInt64() {
    return nil, fmt.Errorf("%w: qfq factor overflow", ErrMalformedResponse)
}
```

- [ ] **Step 5: Run parser tests and verify GREEN**

Run: `go test ./pkg/broker -run 'TestParseSina(Daily|QFQ)' -count=1`

Expected: PASS.

- [ ] **Step 6: Write failing HTTP contract and pacing tests**

Use two `httptest.Server` handlers that validate `symbol`, `scale=240`, `ma=no`, computed `datalen`, factor symbol interpolation, `Referer`, and bounded body handling. With `rate.NewLimiter(rate.Every(20*time.Millisecond), 1)`, record real server arrival times and assert the factor request begins at least 15 ms after the daily request. Return one 503 before success and assert the retry also observes the interval.

- [ ] **Step 7: Implement the source adapter**

Map `SSE -> sh`, `SZSE -> sz`, `BSE -> bj`; reject other exchanges before HTTP. Every attempt must call `limiter.Wait(ctx)` immediately before `client.Do`. Retry only timeout, connection reset, 429, and 5xx at most twice; honor a larger valid `Retry-After`. Use an explicit 15-second client timeout and a 4 MiB body limit.

```go
for attempt := 0; attempt < 3; attempt++ {
    if err := s.limiter.Wait(ctx); err != nil { return nil, err }
    body, retryAfter, err := s.do(ctx, endpoint)
    if err == nil { return body, nil }
    if !retryableSina(err) || attempt == 2 { return nil, err }
    if err := waitContext(ctx, maxDuration(retryAfter, retryDelay(attempt))); err != nil { return nil, err }
}
```

- [ ] **Step 8: Run broker tests and commit**

Run: `go test ./pkg/broker -count=1`

Expected: PASS.

```bash
git add internal/port/market_source.go pkg/broker/sina_market.go pkg/broker/sina_market_test.go
git commit -m "feat: add paced sina equity source"
```

### Task 2: Derive Weekly Bars from Daily Bars

**Files:**
- Create: `internal/market/weekly.go`
- Create: `internal/market/weekly_test.go`

**Interfaces:**
- Consumes: validated `[]market.Bar` with `Timeframe == market.Day` and one instrument.
- Produces:

```go
func AggregateWeekly(id InstrumentID, daily []Bar) ([]Bar, error)
```

- [ ] **Step 1: Write failing aggregation tests**

Use hand-written daily bars for a normal week, a Monday-Wednesday holiday-shortened week, and a current unfinished week. Assert first open, maximum high, minimum low, last close, summed volume/amount, strictest trading status, last daily open/close timestamps, and version zero. Add mixed instrument, wrong timeframe, duplicate date, unsorted input, and invalid daily bar cases.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/market -run TestAggregateWeekly -count=1`

Expected: FAIL because `AggregateWeekly` is undefined.

- [ ] **Step 3: Implement deterministic ISO-week aggregation**

Clone and sort input by close time, reject duplicates and mismatches, group by the `(ISOYear, ISOWeek)` pair, and emit one bar per observed week. Do not assume Friday exists and do not consult the wall clock or a holiday calendar.

```go
year, week := day.CloseTime.ISOWeek()
key := [2]int{year, week}
// first day supplies Open/OpenTime, every day updates High/Low and sums
// Volume/Amount, and the last observed day supplies Close/CloseTime.
```

- [ ] **Step 4: Run and verify GREEN, then commit**

Run: `go test ./internal/market -count=1`

Expected: PASS.

```bash
git add internal/market/weekly.go internal/market/weekly_test.go
git commit -m "feat: derive weekly bars from daily data"
```

### Task 3: Refactor Versioned Stock Ingestion Around Daily Input

**Files:**
- Modify: `internal/application/market_ingestion_service.go`
- Modify: `internal/application/market_ingestion_service_test.go`
- Modify: `internal/application/market_ingestion_final_state_test.go`
- Modify: `internal/infrastructure/mysql/legacy_migrator.go`
- Modify: `internal/infrastructure/mysql/legacy_migrator_test.go`

**Interfaces:**
- Consumes: `port.EquityDailySource.FetchDailyBars`, `FetchAdjustmentFactors`, `market.AggregateWeekly`.
- Produces: unchanged `MarketIngestionService.Refresh(context.Context, market.InstrumentID) (RefreshResult, error)` and unchanged `MarketWriteBatch` persistence boundary.

- [ ] **Step 1: Change fakes and write failing service tests**

Make the fake source expose daily calls and factor calls separately. Add tests proving one refresh makes exactly one logical daily fetch and one factor fetch, derives weekly data from the final merged daily series, inserts earliest-date factor coverage, rejects a newly missing overlapping daily bar, and publishes no corporate actions from the new source.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/application -run 'TestRefresh|TestValidateFinalMarketBars' -count=1`

Expected: build/test failure because `MarketIngestionService` still consumes the old `MarketSource` contract.

- [ ] **Step 3: Implement the daily ingestion flow**

Change the service dependency to `port.EquityDailySource`. Load stored daily bars and factors at the pinned latest version, calculate the existing 20-bar overlap start, fetch raw daily and the complete qfq factor set, merge by close date, call `market.AggregateWeekly`, then publish only changed-window daily and weekly bars plus the complete normalized factor timeline. Ensure factor coverage begins no later than the first retained daily bar.

```go
daily, err := s.source.FetchDailyBars(ctx, id, start, to)
if err != nil { return result, err }
factors, err := s.source.FetchAdjustmentFactors(ctx, id)
if err != nil { return result, err }
finalDaily := mergeDaily(stored[market.Day], daily)
weekly, err := market.AggregateWeekly(id, finalDaily)
batch := port.MarketWriteBatch{Source: s.config.Source, Instrument: id,
    Bars: map[market.Timeframe][]market.Bar{market.Day: daily, market.Week: changedWeekly(weekly, start)},
    Factors: coverFactors(factors, finalDaily[0].CloseTime)}
```

- [ ] **Step 4: Keep legacy migration explicit**

Give the legacy migrator its own narrow compatibility source interface or adapter so migration code can still read the immutable Eastmoney fixture contract without forcing production ingestion back to weekly upstream calls. Do not add an automatic production fallback.

- [ ] **Step 5: Run application and migration tests**

Run: `go test ./internal/application ./internal/infrastructure/mysql -count=1`

Expected: PASS except Docker-gated integration tests may skip with their existing explicit message.

- [ ] **Step 6: Commit**

```bash
git add internal/application/market_ingestion_service.go internal/application/market_ingestion_service_test.go internal/application/market_ingestion_final_state_test.go internal/infrastructure/mysql/legacy_migrator.go internal/infrastructure/mysql/legacy_migrator_test.go
git commit -m "refactor: ingest equities from daily source"
```

### Task 4: Wire the Five-Second Production Limiter and Telemetry

**Files:**
- Modify: `config/config.go`
- Modify: `config.example.yaml`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `internal/application/telemetry.go`
- Modify: `internal/application/telemetry_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: `broker.NewSinaMarketSource(*rate.Limiter)`.
- Produces: production source `sina`, default `StockRequestIntervalSeconds: 5`, structured stock-refresh summaries.

- [ ] **Step 1: Write failing configuration and composition tests**

Add tests that zero config resolves to exactly five seconds, values below five seconds are rejected, and the production kernel reports source `sina`. Test behavior through resolved settings and a constructor seam; do not grep `main.go` source text.

- [ ] **Step 2: Run and verify RED**

Run: `go test . ./internal/application -run 'Test.*(Sina|StockRequest|Telemetry)' -count=1`

Expected: FAIL because the new configuration and source wiring do not exist.

- [ ] **Step 3: Implement configuration and wiring**

Add the exact configuration below, default it to 5, reject 1-4, and construct one `rate.Limiter` in `newKernel`. Inject `broker.NewSinaMarketSource(limiter)` and set `MarketIngestionConfig.Source` to `sina`. Keep `ScanBatchSize` for local workers only.

```go
type Config struct {
    DB DB `yaml:"DB"`
    Worker WorkerConfig `yaml:"Worker"`
    Market MarketConfig `yaml:"Market"`
}
type MarketConfig struct {
    StockRequestIntervalSeconds int `yaml:"StockRequestIntervalSeconds"`
}
```

- [ ] **Step 4: Add structured summary telemetry**

At the scheduler boundary log one completion record with `component=market_scheduler`, `operation=stock_refresh`, total/succeeded/failed/duration fields and a stable error code. Do not log each five-second wait or response content.

- [ ] **Step 5: Update project documentation**

Update the architecture and startup configuration in `AGENTS.md` to state that stock history is Sina-paced and weekly bars are local. Update `config.example.yaml` comments with the 5-second lower bound.

- [ ] **Step 6: Run focused tests and commit**

Run: `go test . ./internal/application ./pkg/broker -count=1`

Expected: PASS.

```bash
git add config/config.go config.example.yaml main.go main_test.go internal/application/telemetry.go internal/application/telemetry_test.go AGENTS.md
git commit -m "feat: switch equity ingestion to sina"
```

### Task 5: Verify the Stock Cutover

**Files:**
- Modify: `api/api.md`
- Modify: `internal/infrastructure/mysql/README.md`

**Interfaces:**
- Consumes: completed Tasks 1-4.
- Produces: a tested, documented stock-source cutover ready for the futures plan.

- [ ] **Step 1: Run package coverage and fix uncovered behavior with new RED/GREEN cycles**

Run:

```bash
go test ./pkg/broker ./internal/market ./internal/application -coverprofile=/tmp/sina-equity.cover
go tool cover -func=/tmp/sina-equity.cover
```

Expected: changed core packages at least 90%; add behavior tests before any production fix.

- [ ] **Step 2: Run repository fast checks**

Run: `go test ./... && go vet ./...`

Expected: PASS.

- [ ] **Step 3: Run full verification**

Run: `bash scripts/verify.sh`

Expected: all unit, race, coverage, performance, MySQL 5.7/8.0, and image gates pass; if Docker is unavailable, record exactly which Docker-only gates could not run.

- [ ] **Step 4: Perform one manual low-frequency smoke test**

Fetch `SSE:600000` raw daily and qfq factors once through the new adapter, verify the latest date is present, and compare one historical adjusted close using `raw/f`. Do not run an all-market public smoke test during development.

- [ ] **Step 5: Commit documentation or verification fixes**

```bash
git add api/api.md internal/infrastructure/mysql/README.md
git commit -m "docs: document sina market ingestion"
```
