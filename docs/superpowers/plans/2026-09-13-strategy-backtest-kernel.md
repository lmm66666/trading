# Go Strategy and Backtest Kernel Migration Implementation Plan

> 历史计划（已废止）：其中的 MySQL 容器、版本矩阵和验收指令已由 [REQ-2026-002](../../changes/archive/legacy/REQ-2026-002-mysql8-cross-architecture.md)与[MySQL 设计](../../design/internal/infrastructure/mysql.md)取代，不得直接执行本文的旧步骤。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the legacy filter-based signal engine with a deterministic Go strategy and backtest kernel, versioned market data, durable runs, and snapshot-based full-market scanning.

**Architecture:** Pure `internal/market`, `internal/indicator`, `internal/strategy`, and `internal/backtest` packages form the domain kernel. Application services depend on ports, while MySQL/GORM and Gin provide adapters; vectorized features are computed once and strategies plus execution advance bar-by-bar without future access.

**Tech Stack:** Go 1.25.7+, Gin, GORM, MySQL 8.4.x LTS, `testing` with Testify 1.11.1, remote isolated integration tests.

**Spec:** `docs/superpowers/specs/2026-09-13-strategy-backtest-kernel-design.md`

## Global Constraints

- Do not use git worktrees; execute in the current checkout on a `codex/` branch.
- Preserve the user's existing `.claude`, `.gitignore`, `.claude/worktrees`, and `.tmp` changes and never stage them implicitly.
- Before reading or changing a directory, read its local `README.md` when present and follow its constraints.
- Use TDD for every behavior change; domain kernel coverage must be at least 90% and repository-wide coverage at least 80%.
- Use the approved remote MySQL 8.4.x target; do not rely on window functions or `SKIP LOCKED` without a separately approved design change.
- Strategies are trusted Go code compiled into the service; never accept or compile uploaded source.
- Indicator decisions use forward-adjusted data; execution and accounting use raw prices plus corporate actions.
- Signals are evaluated after the current close and orders are filled no earlier than the next bar open.
- Keep a single runtime strategy engine after cutover; no long-lived compatibility engine.
- Prices and money use scaled integer value objects for accounting; `float64` is limited to feature math and reported ratios.
- The `/commit-commands:commit` command is unavailable in this environment; use scoped `git add` and English `git commit` messages shown below.

---

## File Map

| Area | Files | Responsibility |
|---|---|---|
| Market domain | `internal/market/*.go` | Instruments, timeframes, fixed-point values, bars, data versions, datasets, adjustments, actions |
| Indicator domain | `internal/indicator/*.go` | Validity-aware series, feature references, feature graph and indicator calculations |
| Strategy domain | `internal/strategy/*.go`, `internal/strategy/builtin/*.go` | Strategy contract, registry, context, exit policy and three built-in Go strategies |
| Backtest domain | `internal/backtest/*.go` | Orders, execution, costs, account, corporate actions, engine and performance metrics |
| Ports/application | `internal/port/*.go`, `internal/application/*.go` | Data/run/queue/cache boundaries and backtest/scan use cases |
| MySQL adapter | `internal/infrastructure/mysql/*.go`, `internal/infrastructure/mysql/dbtest/*.go` | GORM rows, versioned repositories, run storage, leases and snapshots |
| External data | `pkg/broker/eastmoney_market.go` | Raw/QFQ K-lines and dividend/bonus action ingestion |
| Transport | `api/*backtest*.go`, `api/*scan*.go`, `api/router.go`, `api/handler.go` | Async v1 endpoints and existing path adapters |
| Migration | `cmd/migrate-strategy-kernel/main.go` | Validated copy from legacy daily/weekly/stock-info tables |
| Financial screening | `internal/financialscreen/*.go` | Existing report filters moved outside the technical strategy engine |
| Composition/docs | `main.go`, `config/config.go`, `Dockerfile`, `.dockerignore`, `model/README.md`, `api/api.md`, `README.md` | Wiring, safe configuration and current documentation |

---

### Task 1: Market Domain and Data Quality Boundary

**Files:**
- Create: `internal/market/instrument.go`
- Create: `internal/market/timeframe.go`
- Create: `internal/market/value.go`
- Create: `internal/market/bar.go`
- Create: `internal/market/dataset.go`
- Create: `internal/market/adjustment.go`
- Create: `internal/market/corporate_action.go`
- Test: `internal/market/dataset_test.go`
- Test: `internal/market/adjustment_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: standard library only.
- Produces: `market.InstrumentID`, `market.Timeframe`, `market.Price`, `market.Money`, `market.Bar`, `market.Dataset`, `market.DataVersion`, `market.AdjustmentFactor`, `market.CorporateAction`.

- [ ] **Step 1: Write failing value and dataset tests**

Add `github.com/stretchr/testify v1.11.1` as a direct test dependency, then use `assert`/`require` consistently in the new domain tests.

```go
func TestNewDatasetSortsRejectsDuplicatesAndInvalidOHLC(t *testing.T) {
    id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
    bars := []market.Bar{
        testBar(id, "2026-01-06", 102000, 105000, 101000, 104000),
        testBar(id, "2026-01-05", 100000, 103000, 99000, 102000),
    }
    got, err := market.NewDataset(id, market.Day, 7, bars)
    require.NoError(t, err)
    assert.True(t, got.Bar(0).CloseTime.Before(got.Bar(1).CloseTime))

    _, err = market.NewDataset(id, market.Day, 7, append(bars, bars[0]))
    assert.ErrorIs(t, err, market.ErrDuplicateBar)
}

func TestInstrumentIDRejectsAmbiguousCode(t *testing.T) {
    _, err := market.ParseInstrumentID("000001")
    assert.ErrorIs(t, err, market.ErrExchangeRequired)
}
```

- [ ] **Step 2: Run the market tests and confirm the package is missing**

Run: `go test ./internal/market -run 'Test(NewDataset|InstrumentID)'`

Expected: FAIL because `internal/market` does not exist.

- [ ] **Step 3: Implement fixed-point values, bars, and validated datasets**

```go
type Price int64
type Money int64
type DataVersion uint64

type PriceView uint8
const (
    Raw PriceView = iota
    ForwardAdjusted
)

type TradingStatus uint8
const (
    Tradable TradingStatus = iota
    Suspended
)

const ValueScale int64 = 10_000

type Bar struct {
    Instrument   InstrumentID
    Timeframe    Timeframe
    OpenTime     time.Time
    CloseTime    time.Time
    Open, High   Price
    Low, Close   Price
    Volume       int64
    Amount       Money
    Trading      TradingStatus
    LimitUp      *Price
    LimitDown    *Price
    Version      DataVersion
}

func NewDataset(id InstrumentID, tf Timeframe, version DataVersion, bars []Bar) (Dataset, error)
func (d Dataset) Len() int
func (d Dataset) Bar(i int) Bar
func (d Dataset) Bars() []Bar // returns a defensive copy
```

Validate exchange/code, matching instrument/timeframe/version, positive OHLC, `low <= open/close <= high`, nonnegative volume, strictly increasing CloseTime, and duplicates after sorting.

- [ ] **Step 4: Add failing adjustment and as-of alignment tests**

```go
func TestAdjustedCloseUsesLatestKnownFactor(t *testing.T) {
    factors := []market.AdjustmentFactor{
        {EffectiveTime: utc("2026-01-01"), Numerator: 8, Denominator: 10},
    }
    got, ok := market.AdjustedPrice(market.Price(100_000), utc("2026-01-02"), factors)
    assert.True(t, ok)
    assert.InDelta(t, 8.0, got, 0.0001)
}

func TestAlignAsOfNeverReadsFutureAuxiliaryBar(t *testing.T) {
    got := market.AlignAsOf(primaryWeekly, auxiliaryDaily)
    assert.Equal(t, utc("2026-01-09"), got[1].AuxiliaryCloseTime)
    assert.True(t, got[1].AuxiliaryCloseTime.Before(got[1].PrimaryCloseTime) || got[1].AuxiliaryCloseTime.Equal(got[1].PrimaryCloseTime))
}
```

- [ ] **Step 5: Implement adjustment factors, company actions, and as-of alignment**

```go
type AdjustmentFactor struct {
    EffectiveTime time.Time
    Numerator     int64
    Denominator   int64
    Version       DataVersion
}

type CorporateAction struct {
    ID            string
    Instrument    InstrumentID
    ExDate        time.Time
    Kind          CorporateActionKind
    CashPerShare  Money
    ShareNumerator int64
    ShareDenominator int64
    Version       DataVersion
}

type CorporateActionKind uint8
const (
    CashDividend CorporateActionKind = iota + 1
    ShareDistribution
    RightsIssue
)

func AdjustedPrice(raw Price, at time.Time, factors []AdjustmentFactor) (float64, bool)
func AlignAsOf(primary, auxiliary Dataset) []Alignment
```

Use binary search for the last auxiliary CloseTime not after the primary CloseTime. Return `ok=false` when no factor applies; never silently substitute raw prices.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/market -cover`

Expected: PASS with package coverage at least 90%.

```bash
git add internal/market go.mod go.sum
git commit -m "feat: add versioned market domain primitives"
```

---

### Task 2: Validity-Aware Indicator Engine and Feature Graph

**Files:**
- Create: `internal/indicator/series.go`
- Create: `internal/indicator/ref.go`
- Create: `internal/indicator/sma.go`
- Create: `internal/indicator/ema.go`
- Create: `internal/indicator/macd.go`
- Create: `internal/indicator/kdj.go`
- Create: `internal/indicator/graph.go`
- Test: `internal/indicator/series_test.go`
- Test: `internal/indicator/graph_test.go`
- Test: `internal/indicator/prefix_test.go`

**Interfaces:**
- Consumes: `market.Dataset`, `market.AdjustmentFactor` from Task 1.
- Produces: `indicator.Ref`, `indicator.Series`, `indicator.Set`, `indicator.Build`.

- [ ] **Step 1: Write failing warm-up and prefix-invariance tests**

```go
func TestSMAIsInvalidUntilFullPeriod(t *testing.T) {
    got := indicator.SMA([]float64{1, 2, 3, 4}, 3)
    assert.False(t, got.Valid(0))
    assert.False(t, got.Valid(1))
    assert.Equal(t, 2.0, mustValue(t, got, 2))
}

func TestEveryBuiltInFeatureIsPrefixInvariant(t *testing.T) {
    for _, ref := range builtInRefs() {
        all := mustBuild(t, dataset(160), ref)
        for _, n := range []int{20, 60, 120} {
            prefix := mustBuild(t, dataset(n), ref)
            assertSeriesEqual(t, all.Slice(0, n), prefix)
        }
    }
}
```

- [ ] **Step 2: Run the indicator tests and confirm failure**

Run: `go test ./internal/indicator -run 'Test(SMA|EveryBuiltInFeature)'`

Expected: FAIL because the new indicator package is absent.

- [ ] **Step 3: Implement Series and typed feature references**

```go
type Series struct {
    values []float64
    valid  []bool
}

func (s Series) At(i int) (float64, bool)
func (s Series) Len() int

type Ref struct {
    Kind       Kind
    Timeframe  market.Timeframe
    PriceView market.PriceView
    Field      Field
    Period     int
    Fast       int
    Slow       int
    Signal     int
}

func (r Ref) Key() string
```

Reject nonpositive periods and unstable reference keys. Use full-period SMA warm-up; preserve standard recursive EMA seeding rules in one documented implementation.

- [ ] **Step 4: Implement feature computation and graph-level deduplication**

```go
type Set map[string]Series

func Build(ds market.Dataset, factors []market.AdjustmentFactor, refs []Ref) (Set, error) {
    unique := deduplicate(refs)
    out := make(Set, len(unique))
    for _, ref := range unique {
        series, err := compute(ds, factors, ref)
        if err != nil { return nil, fmt.Errorf("compute %s: %w", ref.Key(), err) }
        out[ref.Key()] = series
    }
    return out, nil
}
```

Implement adjusted/raw OHLC source series, SMA, EMA, volume MA, MACD DIF/DEA/histogram, and KDJ K/D/J. Add an internal compute counter in tests to prove duplicate refs run once.

- [ ] **Step 5: Run indicator tests and legacy comparison fixtures**

Run: `go test ./internal/indicator -cover`

Expected: PASS with prefix-invariance checks and at least 90% coverage.

- [ ] **Step 6: Commit**

```bash
git add internal/indicator
git commit -m "feat: add deterministic indicator feature graph"
```

---

### Task 3: Strategy Contract, Context, and Registry

**Files:**
- Create: `internal/strategy/action.go`
- Create: `internal/strategy/context.go`
- Create: `internal/strategy/definition.go`
- Create: `internal/strategy/strategy.go`
- Create: `internal/strategy/registry.go`
- Create: `internal/strategy/replay.go`
- Test: `internal/strategy/context_test.go`
- Test: `internal/strategy/registry_test.go`
- Test: `internal/strategy/replay_test.go`

**Interfaces:**
- Consumes: `market.Bar`, `market.Timeframe`, `indicator.Ref`, `indicator.Set`.
- Produces: `strategy.Strategy`, `strategy.Context`, `strategy.Decision`, `strategy.Registry`, `strategy.ReplayLatest`.

- [ ] **Step 1: Write failing future-access and registry tests**

```go
func TestContextRejectsFutureAccess(t *testing.T) {
    ctx := newTestContext(5)
    _, ok := ctx.Float(closeRef, -1)
    assert.False(t, ok)
    assert.ErrorIs(t, ctx.Err(), strategy.ErrFutureAccess)
}

func TestRegistryReturnsFreshStrategyInstances(t *testing.T) {
    first, _ := registry.Resolve("daily_b1_buy", "1", nil)
    second, _ := registry.Resolve("daily_b1_buy", "1", nil)
    assert.NotSame(t, first, second)
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run: `go test ./internal/strategy -run 'Test(Context|Registry)'`

Expected: FAIL because the contracts do not exist.

- [ ] **Step 3: Implement the contract and immutable context**

```go
type Action uint8
const (
    Hold Action = iota
    EnterLong
    ExitLong
)

type Decision struct {
    Action Action
    Reason string
    Values map[string]float64
}

type Definition struct {
    ID               string
    Version          string
    PrimaryTimeframe market.Timeframe
    WarmupBars       int
    Features         []indicator.Ref
    Auxiliary        []market.Timeframe
    DefaultHoldBars  int
    Parameters       map[string]ParameterSpec
}

type ParameterSpec struct {
    Default float64
    Min     float64
    Max     float64
    Integer bool
}

type Strategy interface {
    Definition() Definition
    OnBar(Context) (Decision, error)
}

var (
    ErrFutureAccess = errors.New("strategy attempted future access")
    ErrUnknownStrategy = errors.New("unknown strategy")
    ErrUnknownParameter = errors.New("unknown strategy parameter")
)

type Context interface {
    Bar() market.Bar
    Index() int
    Float(ref indicator.Ref, ago int) (float64, bool)
    Position() PositionView
    Err() error
}

type Timeline struct {
    Primary   market.Dataset
    Features  indicator.Set
    Auxiliary map[market.Timeframe]AlignedFeatures
}

type AlignedFeatures struct {
    Dataset     market.Dataset
    Features    indicator.Set
    PrimaryToAuxiliary []int
}

func NewTimeline(primary market.Dataset, features indicator.Set, auxiliary map[market.Timeframe]AlignedFeatures) (Timeline, error)
func (t Timeline) Len() int
```

`Context.Float(ref, ago)` calculates `index-ago`, rejects negative `ago`, returns invalid outside the prefix, and exposes only `PositionView{Open, Quantity, HoldingBars}`.

- [ ] **Step 4: Implement validated registry and replay runner**

```go
type Factory func(params map[string]float64) (Strategy, error)

func (r *Registry) Register(id, version string, factory Factory) error
func (r *Registry) Resolve(id, version string, params map[string]float64) (Strategy, error)
func ReplayLatest(st Strategy, timeline Timeline) (Decision, error)
```

Replay creates one fresh stateful strategy instance per Instrument and advances from index zero to the final bar. Reject duplicate registration and unknown parameter names before execution.

After every `OnBar` call, Replay and Engine check `Context.Err()` even when the strategy ignored the `ok` return from `Float`; any attempted future access therefore fails the run instead of becoming a silent Hold.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/strategy -cover`

Expected: PASS with at least 90% coverage.

```bash
git add internal/strategy
git commit -m "feat: define safe Go strategy runtime"
```

---

### Task 4: Forward-Only Built-In Strategy Migration

**Files:**
- Create: `internal/strategy/builtin/register.go`
- Create: `internal/strategy/builtin/pullback_tracker.go`
- Create: `internal/strategy/builtin/daily_b1.go`
- Create: `internal/strategy/builtin/weekly_b1.go`
- Create: `internal/strategy/builtin/bottom_surge.go`
- Test: `internal/strategy/builtin/daily_b1_test.go`
- Test: `internal/strategy/builtin/weekly_b1_test.go`
- Test: `internal/strategy/builtin/bottom_surge_test.go`
- Test: `internal/strategy/builtin/prefix_test.go`

**Interfaces:**
- Consumes: Strategy runtime from Task 3 and Feature refs from Task 2.
- Produces: `builtin.RegisterAll(*strategy.Registry) error` and factories for all three strategy IDs at version `1`.

- [ ] **Step 1: Write failing registration and parameter tests**

```go
func TestRegisterAllProvidesThreeStrategies(t *testing.T) {
    r := strategy.NewRegistry()
    require.NoError(t, builtin.RegisterAll(r))
    for _, id := range []string{"daily_b1_buy", "weekly_b1_buy", "bottom_surge_pullback"} {
        _, err := r.Resolve(id, "1", nil)
        assert.NoError(t, err)
    }
}

func TestDailyB1RejectsUnknownParameter(t *testing.T) {
    _, err := newRegistry(t).Resolve("daily_b1_buy", "1", map[string]float64{"mystery": 1})
    assert.ErrorIs(t, err, strategy.ErrUnknownParameter)
}
```

- [ ] **Step 2: Run registration tests and confirm failure**

Run: `go test ./internal/strategy/builtin -run 'Test(RegisterAll|DailyB1Rejects)'`

Expected: FAIL because built-ins are missing.

- [ ] **Step 3: Implement the forward-only pullback tracker**

```go
type phase uint8
const (
    idle phase = iota
    surge
    rally
    pullback
    complete
    invalid
)

type pullbackTracker struct {
    phase phase
    surgeIndex int
    lastSurgeIndex int
    peakIndex int
    peakClose float64
}

func (p *pullbackTracker) Advance(input trackerInput) trackerOutput
```

On each call read only the current input and retained past state. Never pass a complete Bar slice into the tracker. Expire windows as soon as maximum pullback depth or bars is exceeded.

- [ ] **Step 4: Implement the three factories with exact defaults**

```go
func NewDailyB1(params map[string]float64) (strategy.Strategy, error)
func NewWeeklyB1(params map[string]float64) (strategy.Strategy, error)
func NewBottomSurge(params map[string]float64) (strategy.Strategy, error)
```

Use these defaults:

- Daily B1: volume ratio 2.0, rally 5%, pullback 15%, pullback bars 10, KDJ threshold 40, MA20 trend lookback 10.
- Weekly B1: KDJ threshold 10, weekly MA20 above MA60, close above weekly MA60, close above aligned daily MA20.
- Bottom surge: 60-bar low band 15%, single-day 2.0/5%, gradual 3 days at 1.2/2%, surge gap 3, pullback 20%/15 bars, MA20 above MA60, close above MA60, J in [-20, 20].

- [ ] **Step 5: Add behavior and no-future golden tests**

```go
func TestBottomSurgePastSignalsDoNotChangeWhenFutureBarsAreAppended(t *testing.T) {
    base := fixtureBottomSurge(90)
    before := replayAll(t, newBottomSurge(t), base)
    after := replayAll(t, newBottomSurge(t), append(base, futureRallyBars()...))
    assert.Equal(t, before, after[:len(before)])
}

func TestWeeklyB1UsesDailyValueAtOrBeforeWeeklyClose(t *testing.T) {
    decision := replayWeekly(t, dailyWithLargeMoveAfterFriday())
    assert.Equal(t, strategy.Hold, decision.Action)
}
```

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/strategy/... -cover`

Expected: PASS; appended future bars never alter earlier decisions.

```bash
git add internal/strategy/builtin
git commit -m "feat: migrate built-in strategies to forward runtime"
```

---

### Task 5: Deterministic Order, Cost, and Account Model

**Files:**
- Create: `internal/backtest/config.go`
- Create: `internal/backtest/order.go`
- Create: `internal/backtest/fill.go`
- Create: `internal/backtest/cost.go`
- Create: `internal/backtest/account.go`
- Create: `internal/backtest/execution.go`
- Test: `internal/backtest/account_test.go`
- Test: `internal/backtest/execution_test.go`
- Test: `internal/backtest/corporate_action_test.go`

**Interfaces:**
- Consumes: fixed-point market values and corporate actions from Task 1.
- Produces: `backtest.Config`, `backtest.Order`, `backtest.Fill`, `backtest.Account`, `backtest.ExecutionModel`.

- [ ] **Step 1: Write failing next-open and board-lot tests**

```go
func TestBuyUsesNextBarOpenAndRoundsToBoardLot(t *testing.T) {
    cfg := backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100}
    order := backtest.NewNextOpenOrder(backtest.Buy, closeTime("2026-01-05"), "signal")
    fill := mustExecute(t, cfg, order, bar("2026-01-06", 123_400, 120_000, 126_000, 10_000))
    assert.EqualValues(t, 800, fill.Quantity)
    assert.Equal(t, market.Price(123_400), fill.Price)
}

func TestOrderDoesNotFillOnSignalBar(t *testing.T) {
    _, reason := execute(orderAt("2026-01-05"), bar("2026-01-05", 100_000, 99_000, 101_000, 1000))
    assert.Equal(t, backtest.RejectNotYetActive, reason)
}
```

- [ ] **Step 2: Run execution tests and confirm failure**

Run: `go test ./internal/backtest -run 'Test(BuyUses|OrderDoesNot)'`

Expected: FAIL because the execution model is absent.

- [ ] **Step 3: Implement validated configuration, costs, and fills**

```go
type Config struct {
    InitialCash       market.Money
    CashFractionBPS   int64
    CommissionBPS     int64
    MinimumCommission market.Money
    StampDutyBPS      int64
    TransferFeeBPS    int64
    SlippageBPS       int64
    LotSize           int64
    HoldBars          int
}

func (c Config) Validate() error
func (m ExecutionModel) Execute(order Order, bar market.Bar, account Account) (Fill, RejectReason)
```

Apply buy slippage upward and sell slippage downward, clamp within Bar Low/High, calculate each fee with integer rounding, and ensure cash never becomes negative.

- [ ] **Step 4: Add failing suspension, limit, and company-action tests**

```go
func TestOneBarEntryExpiresWhenSuspended(t *testing.T) {
    _, reason := execute(entryOrder(), suspendedBar())
    assert.Equal(t, backtest.RejectNotTradable, reason)
}

func TestCashDividendAndShareBonusApplyBeforeOpen(t *testing.T) {
    account := longAccount(1000, 100_000)
    account.Apply([]market.CorporateAction{
        cashDividend(1_000),
        shareRatio(12, 10),
    })
    assert.EqualValues(t, 1200, account.Position().Quantity)
    assert.Equal(t, market.Money(1_000_000), account.CashDelta())
}
```

- [ ] **Step 5: Implement execution rejection and company-action application**

Reject zero-volume/non-trading Bars, buys at known LimitUp, sells at known LimitDown, insufficient cash, invalid lot size, and duplicate fills. Apply cash/share actions once by action ID and return an error for unsupported rights issues.

```go
func (m ExecutionModel) canFill(order Order, bar market.Bar) RejectReason
func (a *Account) ApplyCorporateActions(actions []market.CorporateAction) error
func (a *Account) ApplyFill(fill Fill) error
```

Keep an `appliedActionIDs map[string]struct{}` inside one engine Run. A repeated action ID returns `ErrDuplicateCorporateAction`; `RightsIssue` returns `ErrUnsupportedCorporateAction` before cash or shares change.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/backtest -cover`

Expected: PASS with exact integer cash assertions.

```bash
git add internal/backtest
git commit -m "feat: add deterministic execution and account model"
```

---

### Task 6: Bar-by-Bar Backtest Engine and Metrics

**Files:**
- Create: `internal/backtest/engine.go`
- Create: `internal/backtest/context.go`
- Create: `internal/backtest/result.go`
- Create: `internal/backtest/metrics.go`
- Test: `internal/backtest/engine_test.go`
- Test: `internal/backtest/holding_test.go`
- Test: `internal/backtest/metrics_test.go`

**Interfaces:**
- Consumes: `strategy.Strategy`, timeline/features from Tasks 2-4, account/execution from Task 5.
- Produces: `backtest.Engine.Run(context.Context, backtest.Input) (backtest.Result, error)`.

- [ ] **Step 1: Write failing event-order and hold-bar tests**

```go
func TestEngineFillsSignalAtNextOpen(t *testing.T) {
    result := runScripted(t, []strategy.Action{strategy.EnterLong, strategy.Hold})
    assert.Equal(t, date("2026-01-06"), result.Fills[0].Time)
}

func TestDefaultExitCountsEntryBarAsHoldingBarOne(t *testing.T) {
    result := runTenBarHold(t)
    assert.Equal(t, closeTime("2026-01-14"), result.Orders[1].CreatedAt)
    assert.Equal(t, openTime("2026-01-15"), result.Fills[1].Time)
}

func TestLastBarDecisionRemainsUnfilled(t *testing.T) {
    result := runEntryOnlyOnLastBar(t)
    assert.Empty(t, result.Fills)
    assert.Equal(t, backtest.UnfilledNoNextBar, result.Orders[0].FinalReason)
}
```

- [ ] **Step 2: Run engine tests and confirm failure**

Run: `go test ./internal/backtest -run 'Test(Engine|DefaultExit|LastBar)'`

Expected: FAIL because `Engine.Run` is missing.

- [ ] **Step 3: Implement the exact engine clock**

```go
type Input struct {
    Strategy strategy.Strategy
    Timeline strategy.Timeline
    Actions  []market.CorporateAction
    Config   Config
}

func (e Engine) Run(ctx context.Context, input Input) (Result, error) {
    for i := 0; i < input.Timeline.Len(); i++ {
        if err := ctx.Err(); err != nil { return Result{}, err }
        e.applyActionsBeforeOpen(i)
        e.tryPendingOrderAtOpen(i)
        e.markToMarketAtClose(i)
        decision, err := input.Strategy.OnBar(e.contextAt(i))
        if err != nil { return Result{}, fmt.Errorf("strategy bar %d: %w", i, err) }
        e.queueForNextOpen(decision, i)
    }
    return e.finishWithoutSyntheticClose(), nil
}
```

Check `ctx.Err()` before every Bar mutation. This gives Task 13 deterministic cancellation without introducing a second engine API.

If an exit order is rejected, the context continues exposing a held position and the hold policy emits ExitLong again at subsequent closes. Entry rejection expires after one Bar.

- [ ] **Step 4: Write and implement metrics tests**

```go
func TestMetricsExcludeOpenTradeFromWinRate(t *testing.T) {
    got := backtest.CalculateMetrics(equity, []backtest.Trade{winner(), loser()}, openPosition())
    assert.Equal(t, 0.5, got.WinRate)
    assert.Equal(t, 2, got.ClosedTrades)
    assert.True(t, got.HasOpenPosition)
}

func TestMaximumDrawdown(t *testing.T) {
    got := backtest.MaximumDrawdown([]market.Money{100, 120, 90, 110})
    assert.InDelta(t, 0.25, got, 1e-9)
}
```

Define `Summary` with total return, optional annualized return, maximum drawdown, closed-trade count, optional win rate, optional profit factor, average holding Bars, and open-position state. `Result` contains `Summary`, orders, fills, closed trades, equity points, and the final position. Return `nil`/absent metrics where a denominator is undefined instead of NaN or infinity.

- [ ] **Step 5: Run full backtest tests with race detection**

Run: `go test -race ./internal/backtest -cover`

Expected: PASS with at least 90% coverage.

- [ ] **Step 6: Commit**

```bash
git add internal/backtest
git commit -m "feat: add bar-by-bar backtest engine and metrics"
```

---

### Task 7: Application Ports and Stable DTOs

**Files:**
- Create: `internal/port/market_data.go`
- Create: `internal/port/market_data_writer.go`
- Create: `internal/port/market_source.go`
- Create: `internal/port/run_store.go`
- Create: `internal/port/job_queue.go`
- Create: `internal/port/signal_snapshot.go`
- Create: `internal/port/event_bus.go`
- Create: `internal/port/telemetry.go`
- Create: `internal/port/errors.go`
- Create: `internal/application/dto.go`
- Test: `internal/application/dto_test.go`

**Interfaces:**
- Consumes: market and backtest domain result types.
- Produces: stable interfaces used by MySQL adapters and application services in later tasks.

- [ ] **Step 1: Write compile-time contract tests with in-memory fakes**

```go
var _ port.MarketData = (*fakeMarketData)(nil)
var _ port.RunStore = (*fakeRunStore)(nil)
var _ port.JobQueue = (*fakeJobQueue)(nil)
var _ port.SignalSnapshotStore = (*fakeSnapshotStore)(nil)

func TestBacktestDTORejectsUnboundedDateRange(t *testing.T) {
    req := application.BacktestRequest{Start: date("1990-01-01"), End: date("2026-01-01")}
    assert.ErrorIs(t, req.Validate(), application.ErrDateRangeTooLarge)
}
```

- [ ] **Step 2: Define ports with exact signatures**

```go
type MarketData interface {
    LatestCompleteVersion(ctx context.Context) (market.DataVersion, error)
    Dataset(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time, version market.DataVersion) (market.Dataset, []market.AdjustmentFactor, []market.CorporateAction, error)
    BatchDatasets(ctx context.Context, ids []market.InstrumentID, request BatchRequest) (map[market.InstrumentID]Bundle, map[market.InstrumentID]error)
    Instruments(ctx context.Context, scope InstrumentScope) ([]market.InstrumentID, error)
    DirtyInstruments(ctx context.Context, after, through market.DataVersion) ([]market.InstrumentID, error)
}

type MarketSource interface {
    FetchBars(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error)
    FetchCorporateActions(ctx context.Context, id market.InstrumentID) ([]market.CorporateAction, error)
}

type MarketWriteBatch struct {
    Source string
    Instrument market.InstrumentID
    Bars map[market.Timeframe][]market.Bar
    Factors []market.AdjustmentFactor
    Actions []market.CorporateAction
    Digest string
}

type MarketDataWriter interface {
    Publish(ctx context.Context, batch MarketWriteBatch) (market.DataVersion, error)
}

type RunKind string
type RunStatus string
type DataQuality string

const (
    DataComplete DataQuality = "COMPLETE"
    DataIncomplete DataQuality = "INCOMPLETE"
)

type InstrumentScope struct {
    Exchanges []market.Exchange
    ActiveOnly bool
    Limit int
}

type BatchRequest struct {
    PrimaryTimeframe market.Timeframe
    Auxiliary []market.Timeframe
    From, To time.Time
    Version market.DataVersion
    LookbackBars int
}

type Bundle struct {
    Primary market.Dataset
    Auxiliary map[market.Timeframe]market.Dataset
    Factors []market.AdjustmentFactor
    Actions []market.CorporateAction
    Quality DataQuality
}

type Run struct {
    ID, IdempotencyKey, InputHash string
    Kind RunKind
    Status RunStatus
    StrategyID, StrategyVersion, EngineVersion string
    DataVersion market.DataVersion
    RequestJSON []byte
    LeaseOwner, LeaseToken string
    CancelRequestedAt *time.Time
}

type Failure struct {
    Code, Message string
    Retryable bool
}

const (
    RunPending RunStatus = "PENDING"
    RunRunning RunStatus = "RUNNING"
    RunSucceeded RunStatus = "SUCCEEDED"
    RunPartialSucceeded RunStatus = "PARTIAL_SUCCEEDED"
    RunFailed RunStatus = "FAILED"
    RunCancelled RunStatus = "CANCELLED"
)

type JobQueue interface {
    Enqueue(ctx context.Context, run Run) (Run, error)
    Claim(ctx context.Context, owner string, lease time.Duration) (Run, error)
    Renew(ctx context.Context, runID, leaseToken string, lease time.Duration) error
    Retry(ctx context.Context, runID, leaseToken string, nextAttempt time.Time, failure Failure) error
    RequestCancel(ctx context.Context, runID string) error
}

type RunStore interface {
    CompleteBacktest(ctx context.Context, runID, leaseToken string, result backtest.Result) error
    CompleteScan(ctx context.Context, runID, leaseToken string, snapshot SignalSnapshot) error
    Fail(ctx context.Context, runID, leaseToken string, failure Failure) error
    Get(ctx context.Context, runID string) (Run, error)
    BacktestResult(ctx context.Context, runID string) (backtest.Summary, error)
    Orders(ctx context.Context, runID string, page PageRequest) (Page[backtest.Order], error)
    Trades(ctx context.Context, runID string, page PageRequest) (Page[backtest.Fill], error)
    Equity(ctx context.Context, runID string, page PageRequest) (Page[backtest.EquityPoint], error)
}

type SignalSnapshotStore interface {
    Latest(ctx context.Context, key SnapshotKey, page PageRequest) (SignalSnapshot, error)
}

type EventBus interface {
    Publish(ctx context.Context, event Event) error
}

type StageObservation struct {
    RunID, Stage, StrategyID, StrategyVersion string
    Instrument *market.InstrumentID
    DataVersion market.DataVersion
    Duration time.Duration
    Rows, Success, Failed, Skipped int64
}

type Telemetry interface {
    ObserveStage(ctx context.Context, observation StageObservation)
    CountRetry(ctx context.Context, runID, errorCode string, attempt int)
}

type SnapshotKey struct {
    StrategyID, StrategyVersion string
    ParametersHash string
    AsOf time.Time
}

type SnapshotRow struct {
    Instrument market.InstrumentID
    SignalTime time.Time
    Reason string
    Values map[string]float64
}

type SignalSnapshot struct {
    ID, RunID string
    Key SnapshotKey
    DataVersion market.DataVersion
    Rows []SnapshotRow
    Failures map[market.InstrumentID]Failure
}

type PageRequest struct {
    AfterSequence int64
    Limit int
}

type Page[T any] struct {
    Items []T
    NextSequence *int64
}

type Event struct {
    ID, Kind, AggregateID string
    Payload []byte
    OccurredAt time.Time
}

// internal/application/dto.go
type BacktestRequest struct {
    Instrument market.InstrumentID
    StrategyID, StrategyVersion, IdempotencyKey string
    Parameters map[string]float64
    Start, End time.Time
    Config backtest.Config
}

var ErrInvalidRequest = errors.New("invalid application request")

// internal/port/errors.go
var (
    ErrTemporary = errors.New("temporary infrastructure failure")
    ErrSnapshotNotReady = errors.New("signal snapshot not ready")
    ErrRunNotFound = errors.New("run not found")
    ErrLeaseLost = errors.New("run lease lost")
)
```

Add typed request limits: maximum 20 years per single-stock backtest, maximum 5000 instruments per scan, and maximum page size 1000. A zero `SnapshotKey.AsOf` means the newest published snapshot matching strategy ID/version/parameter hash.

- [ ] **Step 3: Run tests and commit**

Run: `go test ./internal/application ./internal/port`

Expected: PASS.

```bash
git add internal/application internal/port
git commit -m "feat: define strategy application ports"
```

---

### Task 8: MySQL Schema and Versioned Market Repository

**Files:**
- Create: `internal/infrastructure/mysql/base_model.go`
- Create: `internal/infrastructure/mysql/instrument_model.go`
- Create: `internal/infrastructure/mysql/data_version_model.go`
- Create: `internal/infrastructure/mysql/market_bar_model.go`
- Create: `internal/infrastructure/mysql/adjustment_factor_model.go`
- Create: `internal/infrastructure/mysql/corporate_action_model.go`
- Create: `internal/infrastructure/mysql/compute_run_model.go`
- Create: `internal/infrastructure/mysql/backtest_run_model.go`
- Create: `internal/infrastructure/mysql/backtest_order_model.go`
- Create: `internal/infrastructure/mysql/backtest_trade_model.go`
- Create: `internal/infrastructure/mysql/backtest_equity_model.go`
- Create: `internal/infrastructure/mysql/signal_snapshot_model.go`
- Create: `internal/infrastructure/mysql/signal_snapshot_row_model.go`
- Create: `internal/infrastructure/mysql/outbox_event_model.go`
- Create: `internal/infrastructure/mysql/migrate.go`
- Create: `internal/infrastructure/mysql/market_data_repo.go`
- Create: `internal/infrastructure/mysql/market_data_writer.go`
- Create: `internal/infrastructure/mysql/dbtest/mysql.go`
- Test: `internal/infrastructure/mysql/migrate_integration_test.go`
- Test: `internal/infrastructure/mysql/market_data_repo_integration_test.go`
- Test: `internal/infrastructure/mysql/market_data_writer_integration_test.go`
- Modify: `data/data.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `port.MarketData` and market types.
- Produces: `mysql.MarketDataRepository` implementing `port.MarketData` and `port.MarketDataWriter`; schema creation for all new tables.

- [ ] **Step 1: Add remote isolated MySQL test bootstrap and failing schema tests**

```go
func TestMigrateCreatesKernelTablesOnMySQL84(t *testing.T) {
    for _, target := range dbtest.Targets() {
        t.Run(target, func(t *testing.T) {
            db := dbtest.OpenIsolatedMySQL(t, target)
            require.NoError(t, mysql.Migrate(db))
            assertTables(t, db, "t_instruments", "t_market_bars", "t_compute_runs", "t_signal_snapshots")
        })
    }
}
```

Mark integration files with `//go:build integration`; the fixture reads local `config.yaml`, validates the approved remote target, replaces the configured business database with a random isolated database, and deletes it during cleanup.

- [ ] **Step 2: Run the schema test and confirm failure**

Run: `go test -tags=integration ./internal/infrastructure/mysql -run TestMigrateCreatesKernelTablesOnMySQL57And80 -count=1`

Expected: FAIL because models and migration registration are absent.

- [ ] **Step 3: Implement one GORM model per table with required indexes**

```go
type MarketBarModel struct {
    BaseModel
    InstrumentID      uint64    `gorm:"not null;index:idx_bar_current,priority:3;uniqueIndex:uq_bar_revision,priority:1"`
    Timeframe         string    `gorm:"size:16;not null;index:idx_bar_current,priority:1;uniqueIndex:uq_bar_revision,priority:2"`
    CloseTime         time.Time `gorm:"not null;index:idx_bar_current,priority:2;uniqueIndex:uq_bar_revision,priority:3"`
    Revision          uint32    `gorm:"not null;uniqueIndex:uq_bar_revision,priority:4"`
    ValidFromVersion  uint64    `gorm:"not null;index"`
    ValidToVersion    *uint64   `gorm:"index"`
    Open, High, Low, Close int64
    Volume, Amount    int64
    TradingStatus     string
    LimitUp, LimitDown *int64
}
```

Use the following field matrix for the remaining one-table-per-file models:

| Model | Required business fields |
|---|---|
| `InstrumentModel` | `Exchange`, `Code`, `Name`, `Board`, `Active`, `LotSize`, `Source` |
| `DataVersionModel` | `Version`, `Source`, `Status`, `Quality`, `Digest`, `PublishedAt` |
| `AdjustmentFactorModel` | `InstrumentID`, `EffectiveTime`, `Numerator`, `Denominator`, `ValidFromVersion`, `ValidToVersion` |
| `CorporateActionModel` | `SourceEventID`, `InstrumentID`, `ExDate`, `Kind`, `CashPerShare`, `ShareNumerator`, `ShareDenominator`, `ValidFromVersion`, `ValidToVersion` |
| `ComputeRunModel` | `RunID`, `Kind`, `Status`, `IdempotencyKey`, `InputHash`, `RequestJSON`, `LeaseOwner`, `LeaseToken`, `LeaseUntil`, `Attempts`, `NextAttemptAt`, `CancelRequestedAt`, `FailureCode`, `FailureMessage` |
| `BacktestRunModel` | `RunID`, `InstrumentID`, `StrategyID`, `StrategyVersion`, `EngineVersion`, `DataVersion`, `ParametersJSON`, `ConfigJSON`, all summary metrics, `HasOpenPosition` |
| `BacktestOrderModel` | `RunID`, `Sequence`, `Side`, `CreatedAtBar`, `ActiveAtBar`, `Quantity`, `Status`, `FinalReason` |
| `BacktestTradeModel` | `RunID`, `Sequence`, `OrderSequence`, `Time`, `Side`, `Quantity`, `Price`, fee columns |
| `BacktestEquityModel` | `RunID`, `Sequence`, `Time`, `Cash`, `PositionValue`, `Equity`, `Drawdown` |
| `SignalSnapshotModel` | `SnapshotID`, strategy identity, `ParametersHash`, `DataVersion`, `AsOf`, `Status`, success/failure counts, `FailuresJSON` |
| `SignalSnapshotRowModel` | `SnapshotID`, `InstrumentID`, `Sequence`, `SignalTime`, `Reason`, `ValuesJSON` |
| `OutboxEventModel` | `EventID`, `Kind`, `AggregateID`, `Payload`, `OccurredAt`, `PublishedAt`, `Attempts` |

All monetary and price columns are signed `BIGINT`; version and sequence columns are unsigned integers. JSON payloads use MySQL `JSON` where 5.7 supports it. Add unique indexes exactly as described in the design and add `(valid_from_version, instrument_id)` indexes so `DirtyInstruments` can find Bars, factors, or actions revised between two versions without scanning historical rows.

`Migrate` calls `AutoMigrate` in dependency order and verifies required index names after creation. Keep the existing legacy daily/weekly AutoMigrate registration until Task 15 switches every runtime reader and writer; Task 15 then removes that registration so the final application has one canonical market schema.

- [ ] **Step 4: Add failing version-visibility and batch-query tests**

```go
func TestDatasetReadsRevisionVisibleAtRequestedVersion(t *testing.T) {
    version3 := uint64(3)
    seedBarRevision(t, db, mysql.MarketBarModel{ValidFromVersion: 1, ValidToVersion: &version3, Close: 100_000})
    seedBarRevision(t, db, mysql.MarketBarModel{ValidFromVersion: 3, ValidToVersion: nil, Close: 110_000})
    v1 := mustDataset(t, repo, 1)
    v3 := mustDataset(t, repo, 3)
    assert.Equal(t, market.Price(100_000), v1.Bar(0).Close)
    assert.Equal(t, market.Price(110_000), v3.Bar(0).Close)
}

func TestBatchDatasetsUsesBoundedStatementCount(t *testing.T) {
    counter := attachQueryCounter(db)
    _, _ = repo.BatchDatasets(ctx, 5000IDs(), request)
    assert.LessOrEqual(t, counter.Selects(), 8)
}

func TestPublishClosesChangedRevisionAndKeepsUnchangedBar(t *testing.T) {
    v1 := mustPublish(t, writer, batchWithClose(100_000))
    v2 := mustPublish(t, writer, batchWithClose(110_000))
    assert.Equal(t, market.Price(100_000), mustDataset(t, repo, v1).Bar(0).Close)
    assert.Equal(t, market.Price(110_000), mustDataset(t, repo, v2).Bar(0).Close)
    assert.Equal(t, int64(2), countBarRevisions(t, db))
}
```

- [ ] **Step 5: Implement current and historical version queries**

For latest scans query `valid_to_version IS NULL` within a CloseTime range and group rows in Go. For historical V query `valid_from_version <= V AND (valid_to_version IS NULL OR valid_to_version > V)`. Load Bars, adjustment factors, and actions in bounded batches; return per-Instrument errors rather than dropping rows. Implement `DirtyInstruments(after, through)` as the union of Instrument IDs whose Bar, factor, or action has `valid_from_version > after AND valid_from_version <= through`.

```go
func (r *MarketDataRepository) BatchDatasets(ctx context.Context, ids []market.InstrumentID, req port.BatchRequest) (map[market.InstrumentID]port.Bundle, map[market.InstrumentID]error)
func (r *MarketDataRepository) DirtyInstruments(ctx context.Context, after, through market.DataVersion) ([]market.InstrumentID, error)
func (r *MarketDataRepository) Publish(ctx context.Context, batch port.MarketWriteBatch) (market.DataVersion, error)

func visibleAt(db *gorm.DB, version uint64) *gorm.DB {
    return db.Where("valid_from_version <= ? AND (valid_to_version IS NULL OR valid_to_version > ?)", version, version)
}
```

`Publish` canonicalizes and hashes the batch, returns the existing version when the digest is unchanged, otherwise creates one pending version, closes only changed current revisions, inserts replacements, and marks the version complete in one transaction. Any validation or transaction error leaves no published version.

- [ ] **Step 6: Wire schema migration into `data.New` and run integration tests**

```go
if err := mysqlinfra.Migrate(db); err != nil {
    return nil, fmt.Errorf("migrate strategy kernel schema: %w", err)
}
```

Run: `go test -tags=integration ./internal/infrastructure/mysql -count=1`

Expected: PASS on both container versions.

- [ ] **Step 7: Commit**

```bash
git add internal/infrastructure/mysql data/data.go go.mod go.sum
git commit -m "feat: add versioned MySQL market storage"
```

---

### Task 9: Eastmoney Raw/QFQ and Corporate-Action Provider

**Files:**
- Create: `pkg/broker/eastmoney_market.go`
- Create: `pkg/broker/eastmoney_market_test.go`
- Create: `pkg/broker/testdata/eastmoney_kline_raw.json`
- Create: `pkg/broker/testdata/eastmoney_kline_qfq.json`
- Create: `pkg/broker/testdata/eastmoney_dividend.json`
- Modify: `pkg/broker/broker.go`

**Interfaces:**
- Consumes: market domain types and `port.MarketSource` from Task 7.
- Produces: `broker.EastmoneyMarketSource` implementing `port.MarketSource`.

- [ ] **Step 1: Save sanitized response fixtures and write failing parser tests**

```go
func TestParseEastmoneyRawAndQFQProducesFactors(t *testing.T) {
    raw := readFixture(t, "eastmoney_kline_raw.json")
    qfq := readFixture(t, "eastmoney_kline_qfq.json")
    bars, factors, err := broker.ParseEastmoneyKlines(raw, qfq, instrument("SSE", "600000"), 9)
    require.NoError(t, err)
    assert.NotEmpty(t, bars)
    assert.NotEmpty(t, factors)
}

func TestParseDividendConvertsPerTenShares(t *testing.T) {
    actions := mustParseActions(t, "eastmoney_dividend.json")
    assert.Equal(t, market.Money(4_200), actions[0].CashPerShare) // 10派4.20 => 0.42 yuan
}
```

- [ ] **Step 2: Run parser tests and confirm failure**

Run: `go test ./pkg/broker -run 'TestParse(Eastmoney|Dividend)'`

Expected: FAIL because provider functions do not exist.

- [ ] **Step 3: Implement provider requests and strict parsing**

```go
type EastmoneyMarketSource struct {
    client *http.Client
    klineBaseURL string
    dataCenterBaseURL string
}

var _ port.MarketSource = (*EastmoneyMarketSource)(nil)

func (s *EastmoneyMarketSource) FetchBars(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error)
func (s *EastmoneyMarketSource) FetchCorporateActions(ctx context.Context, id market.InstrumentID) ([]market.CorporateAction, error)
```

Use `https://push2his.eastmoney.com/api/qt/stock/kline/get` with `fqt=0` and `fqt=1`, and `https://datacenter-web.eastmoney.com/api/data/v1/get` with `reportName=RPT_SHAREBONUS_DET`. Parse `PRETAX_BONUS_RMB`, `BONUS_RATIO`, `IT_RATIO`, and `EX_DIVIDEND_DATE`; validate raw/QFQ dates match and reject zero denominators or non-finite values. Keep endpoints configurable for fixture servers.

For each date, derive the stored factor as `round(qfq_close/raw_close*100_000_000) / 100_000_000`. Verify QFQ open/high/low are within one source price tick of raw OHLC multiplied by the same factor; a mismatch marks the Instrument dataset incomplete instead of accepting a corrupt factor. Convert per-ten-share action fields to per-share fixed-point values before returning domain objects.

- [ ] **Step 4: Add HTTP behavior tests**

Cover timeout, non-2xx, `success=false`, malformed CSV, missing ex-date, pagination, retry-after, and context cancellation with `httptest.Server`. Limit retry to the service-level policy; the provider performs one bounded request per call.

```go
func TestEastmoneyMarketProviderErrors(t *testing.T) {
    for _, tc := range []struct{name string; status int; body string; want error}{
        {"non-2xx", http.StatusBadGateway, `{}`, broker.ErrUpstream},
        {"success-false", http.StatusOK, `{"success":false}`, broker.ErrUpstream},
        {"malformed", http.StatusOK, `{"data":{"klines":["bad"]}}`, broker.ErrMalformedResponse},
    } {
        t.Run(tc.name, func(t *testing.T) { assert.ErrorIs(t, callFixtureServer(tc), tc.want) })
    }
}
```

- [ ] **Step 5: Run broker tests and commit**

Run: `go test ./pkg/broker -cover`

Expected: PASS without live internet access.

```bash
git add pkg/broker
git commit -m "feat: ingest adjusted prices and corporate actions"
```

---

### Task 10: Legacy Data Migrator and Quality Validation

**Files:**
- Create: `internal/infrastructure/mysql/legacy_migrator.go`
- Create: `internal/infrastructure/mysql/legacy_migrator_test.go`
- Create: `cmd/migrate-strategy-kernel/main.go`
- Test: `cmd/migrate-strategy-kernel/main_test.go`

**Interfaces:**
- Consumes: legacy `t_stock_info`, `t_stock_kline_daily`, `t_stock_kline_weekly`; new versioned repository; providers from Task 9.
- Produces: restartable migration command and a validated initial data version.

- [ ] **Step 1: Write failing mapping and idempotency tests**

```go
func TestLegacyCodeMappingRejectsAmbiguousOrUnknownPrefix(t *testing.T) {
    _, err := mysql.MapLegacyInstrument("123456")
    assert.ErrorIs(t, err, mysql.ErrUnknownExchange)
}

func TestLegacyMigrationIsRestartable(t *testing.T) {
    first := runMigration(t, db)
    second := runMigration(t, db)
    assert.Equal(t, first.Version, second.Version)
    assert.Equal(t, first.BarCount, second.BarCount)
}
```

- [ ] **Step 2: Implement batched copy and checkpointing**

```go
type MigrationReport struct {
    Version         market.DataVersion
    InstrumentCount int64
    DailyBarCount   int64
    WeeklyBarCount  int64
    RejectedCodes   []string
    Quality         port.DataQuality
    Digest          string
}

func (m LegacyMigrator) Run(ctx context.Context, batchSize int) (MigrationReport, error)
```

Map only the known equity prefixes: SSE `600/601/603/605/688/689`, SZSE `000/001/002/003/300/301`, and BSE codes beginning `4`, `8`, or `920`. Reject every other or multiply matched code, copy in primary-key order, checkpoint the last legacy ID, and compute counts/date ranges/SHA-256 digest. Backfill raw/QFQ/actions through Task 9 before marking the version `COMPLETE`; otherwise store `INCOMPLETE` and keep backtests disabled for that version.

- [ ] **Step 3: Implement a dry-run-first command**

`cmd/migrate-strategy-kernel` accepts `-config`, `-dry-run`, and `-batch-size`. Default is dry-run; applying changes requires `-dry-run=false`. Print counts and digests but never DSNs or credentials.

```go
dryRun := flag.Bool("dry-run", true, "validate migration without committing rows")
batchSize := flag.Int("batch-size", 1000, "legacy rows per batch")
configPath := flag.String("config", "config.yaml", "runtime configuration path")
flag.Parse()
report, err := runner.Run(ctx, mysql.MigrationOptions{DryRun: *dryRun, BatchSize: *batchSize})
```

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/infrastructure/mysql ./cmd/migrate-strategy-kernel`

Expected: PASS, including a second identical run.

```bash
git add internal/infrastructure/mysql/legacy_migrator.go internal/infrastructure/mysql/legacy_migrator_test.go cmd/migrate-strategy-kernel
git commit -m "feat: add validated legacy market data migration"
```

---

### Task 11: Unified Market Ingestion, Scheduling, and Price Query

**Files:**
- Create: `internal/application/market_ingestion_service.go`
- Create: `internal/application/market_query_service.go`
- Create: `internal/application/market_scheduler.go`
- Test: `internal/application/market_ingestion_service_test.go`
- Test: `internal/application/market_query_service_test.go`
- Test: `internal/application/market_scheduler_test.go`

**Interfaces:**
- Consumes: `port.MarketSource`, `port.MarketData`, `port.MarketDataWriter`, Task 9 provider, and versioned storage from Task 8.
- Produces: `MarketIngestionService.Refresh`, `MarketQueryService.Prices`, and a bounded scheduler that write/read only the new market schema.

- [ ] **Step 1: Write failing ingestion and version-publication tests**

```go
func TestRefreshPublishesRawAdjustedAndActionsTogether(t *testing.T) {
    source.Raw = dailyAndWeeklyRaw()
    source.Factors = qfqFactors()
    source.Actions = dividendActions()
    result, err := service.Refresh(ctx, instrument("SSE:600000"))
    require.NoError(t, err)
    assert.Equal(t, port.DataComplete, result.Quality)
    assert.Equal(t, 1, writer.PublishCalls)
    assert.NotEmpty(t, writer.LastBatch.Bars[market.Day])
    assert.NotEmpty(t, writer.LastBatch.Bars[market.Week])
    assert.NotEmpty(t, writer.LastBatch.Factors)
}

func TestRefreshDoesNotPublishPartialSourceResponse(t *testing.T) {
    source.WeeklyErr = port.ErrTemporary
    _, err := service.Refresh(ctx, instrument("SSE:600000"))
    assert.ErrorIs(t, err, port.ErrTemporary)
    assert.Equal(t, 0, writer.PublishCalls)
}
```

- [ ] **Step 2: Implement atomic refresh into one data version**

```go
type RefreshResult struct {
    Instrument market.InstrumentID
    Version market.DataVersion
    Quality port.DataQuality
    DailyBars, WeeklyBars int
}

func (s *MarketIngestionService) Refresh(ctx context.Context, id market.InstrumentID) (RefreshResult, error)
```

Fetch raw/QFQ day and week data plus actions, validate them before invoking the writer, and publish one digest-backed `MarketWriteBatch`. For existing Instruments refetch the latest 20 trading days plus any missing range so provider corrections create revisions without downloading full history every day.

- [ ] **Step 3: Write failing price-query tests**

```go
func TestPricesReadExactRequestedVersionAndView(t *testing.T) {
    got, err := query.Prices(ctx, PriceQuery{Instrument: id, Timeframe: market.Day, Version: 7, View: market.ForwardAdjusted})
    require.NoError(t, err)
    assert.Equal(t, market.DataVersion(7), got.DataVersion)
    assert.Equal(t, 8.0, got.Bars[0].Close)
}
```

- [ ] **Step 4: Implement query DTOs without leaking GORM models**

```go
type PriceQuery struct {
    Instrument market.InstrumentID
    Timeframe market.Timeframe
    View market.PriceView
    From, To time.Time
    Version market.DataVersion
    Limit int
}

func (s *MarketQueryService) Prices(ctx context.Context, query PriceQuery) (PriceResult, error)
```

Default a zero version to `LatestCompleteVersion`, limit output to 5000 Bars, return UTC RFC 3339 times at the API boundary, and fail when adjusted prices lack factors.

- [ ] **Step 5: Implement a bounded scheduler with single-flight protection**

```go
func (s *MarketScheduler) RunOnce(ctx context.Context, workers int) RefreshSummary
func (s *MarketScheduler) Start(ctx context.Context, interval time.Duration, workers int)
```

Use one process-local trigger guard plus a fixed worker pool, the existing upstream rate limiter, per-Instrument failure capture, and context cancellation. A second overlapping trigger returns `ErrRefreshAlreadyRunning`; it does not start duplicate work.

- [ ] **Step 6: Run application tests and commit**

Run: `go test -race ./internal/application -run 'Test(Refresh|Prices|MarketScheduler)'`

Expected: PASS; no service imports `gorm.io/gorm` or legacy K-line model types.

```bash
git add internal/application/market_ingestion_service.go internal/application/market_ingestion_service_test.go internal/application/market_query_service.go internal/application/market_query_service_test.go internal/application/market_scheduler.go internal/application/market_scheduler_test.go
git commit -m "feat: add unified market ingestion and queries"
```

---

### Task 12: Durable MySQL Runs, Leases, Results, and Snapshots

**Files:**
- Create: `internal/infrastructure/mysql/job_queue.go`
- Create: `internal/infrastructure/mysql/run_store.go`
- Create: `internal/infrastructure/mysql/signal_snapshot_store.go`
- Create: `internal/infrastructure/mysql/outbox.go`
- Test: `internal/infrastructure/mysql/job_queue_integration_test.go`
- Test: `internal/infrastructure/mysql/run_store_integration_test.go`
- Test: `internal/infrastructure/mysql/signal_snapshot_store_integration_test.go`

**Interfaces:**
- Consumes: ports from Task 7 and schema from Task 8.
- Produces: MySQL implementations of `JobQueue`, `RunStore`, `SignalSnapshotStore`, and `EventBus` outbox.

- [ ] **Step 1: Write failing concurrent lease tests**

```go
func TestConcurrentWorkersClaimRunExactlyOnce(t *testing.T) {
    enqueueOne(t, queue)
    claims := claimConcurrently(t, queue, 20)
    assert.Equal(t, 1, successfulClaims(claims))
}

func TestExpiredLeaseCanBeReclaimed(t *testing.T) {
    run := mustClaim(t, queue, "worker-a", time.Second)
    advanceDBClock(t, 2*time.Second)
    reclaimed := mustClaim(t, queue, "worker-b", time.Minute)
    assert.Equal(t, run.ID, reclaimed.ID)
}
```

- [ ] **Step 2: Implement deterministic ordered claiming**

Use a unique claim token and an atomic ordered update:

```sql
UPDATE t_compute_runs
SET status='RUNNING', lease_owner=?, lease_token=?, lease_until=?, attempts=attempts+1
WHERE (status='PENDING' OR (status='RUNNING' AND lease_until < UTC_TIMESTAMP(6)))
  AND cancel_requested_at IS NULL
  AND attempts < 4
  AND (next_attempt_at IS NULL OR next_attempt_at <= UTC_TIMESTAMP(6))
ORDER BY created_at, id
LIMIT 1
```

Then select by the unique lease token. Do not use `SKIP LOCKED`. Renew, retry, and complete require matching run ID plus owner/token so a stale worker cannot overwrite a reclaimed task. `Retry` clears the lease, records the failure, sets `next_attempt_at`, and returns the Run to `PENDING`; a reaper marks expired Runs with four attempts as `FAILED`—one initial attempt plus at most three automatic retries.

- [ ] **Step 3: Write failing atomic-publication and idempotency tests**

```go
func TestSnapshotIsInvisibleUntilPublicationCommits(t *testing.T) {
    tx := beginSnapshot(t, store)
    insertRows(t, tx, 100)
    _, err := store.Latest(ctx, key)
    assert.ErrorIs(t, err, port.ErrSnapshotNotReady)
    commitSnapshot(t, tx)
    assert.Len(t, mustLatest(t, store, key).Rows, 100)
}

func TestDuplicateIdempotencyKeyReturnsExistingRun(t *testing.T) {
    first := mustEnqueue(t, queue, "same-key")
    second := mustEnqueue(t, queue, "same-key")
    assert.Equal(t, first.ID, second.ID)
}
```

- [ ] **Step 4: Implement batched result storage and outbox transaction**

Store orders, fills/trades, and equity points in batches of at most 1000 rows. Complete result and outbox event in one transaction, then mark the run terminal. Snapshot publication uses the same transaction boundary.

```go
func (s *RunStore) CompleteScan(ctx context.Context, runID, leaseToken string, snapshot port.SignalSnapshot) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := insertSnapshot(tx, snapshot); err != nil { return err }
        if err := insertOutbox(tx, scanCompletedEvent(runID, snapshot.ID)); err != nil { return err }
        result := tx.Model(&ComputeRunModel{}).
            Where("run_id = ? AND lease_token = ? AND status = 'RUNNING'", runID, leaseToken).
            Updates(map[string]any{"status": "SUCCEEDED", "lease_token": "", "lease_until": nil})
        if result.Error != nil { return result.Error }
        if result.RowsAffected != 1 { return port.ErrLeaseLost }
        return nil
    })
}
```

Use `PARTIAL_SUCCEEDED` instead of `SUCCEEDED` when `snapshot.Failures` is non-empty. `CompleteBacktest` uses the same token-checked transaction for summary, orders, trades, equity points, outbox, and terminal status.

- [ ] **Step 5: Run integration tests and commit**

Run: `go test -race -tags=integration ./internal/infrastructure/mysql -count=1`

Expected: PASS on the approved remote MySQL 8.4.x isolated database.

```bash
git add internal/infrastructure/mysql
git commit -m "feat: add durable compute runs and snapshots"
```

---

### Task 13: Backtest and Scan Application Services

**Files:**
- Create: `internal/application/backtest_service.go`
- Create: `internal/application/backtest_worker.go`
- Create: `internal/application/scan_service.go`
- Create: `internal/application/scan_worker.go`
- Create: `internal/application/retry.go`
- Create: `internal/application/worker_pool.go`
- Create: `internal/application/telemetry.go`
- Test: `internal/application/backtest_service_test.go`
- Test: `internal/application/scan_service_test.go`
- Test: `internal/application/worker_pool_test.go`
- Test: `internal/application/retry_test.go`
- Test: `internal/application/telemetry_test.go`

**Interfaces:**
- Consumes: Registry, Engine, ports and MySQL-independent DTOs.
- Produces: `BacktestService`, `ScanService`, worker handlers and bounded scan concurrency.

- [ ] **Step 1: Write failing backtest orchestration tests**

```go
func TestCreateBacktestLocksAllReproductionInputs(t *testing.T) {
    run := mustCreateBacktest(t, service, request)
    assert.Equal(t, "weekly_b1_buy", run.StrategyID)
    assert.Equal(t, "1", run.StrategyVersion)
    assert.NotZero(t, run.DataVersion)
    assert.NotEmpty(t, run.EngineVersion)
    assert.NotEmpty(t, run.InputHash)
}

func TestIncompleteDatasetFailsBeforeEngineRuns(t *testing.T) {
    marketData.Quality = port.DataIncomplete
    _, err := service.Execute(ctx, runID)
    assert.ErrorIs(t, err, application.ErrIncompleteMarketData)
    assert.Zero(t, engine.RunCalls)
}
```

- [ ] **Step 2: Implement backtest create/execute/cancel flow**

Resolve and validate strategy at creation, lock latest complete data version, compute a canonical SHA-256 input hash, enqueue idempotently, and return Run. Execution loads the exact version, builds features/timeline, calls `Engine.Run(ctx, input)`, and commits results atomically. The engine checks cancellation before every Bar; scan workers check it between Instruments and repository batches.

```go
func (s *BacktestService) Create(ctx context.Context, req BacktestRequest) (port.Run, error)
func (s *BacktestService) Execute(ctx context.Context, run port.Run) error
func (s *BacktestService) Cancel(ctx context.Context, runID string) error

func canonicalInputHash(req BacktestRequest, definition strategy.Definition, dataVersion market.DataVersion, engineVersion string) (string, error)
```

Canonical JSON sorts parameter keys, uses RFC 3339 UTC timestamps, and includes every cost/execution setting before SHA-256 hashing.

- [ ] **Step 3: Write failing scan query-count and partial-success tests**

```go
func TestScanLoadsEachTimeframeInBoundedBatches(t *testing.T) {
    result := mustScan(t, service, 5000IDs())
    assert.LessOrEqual(t, marketData.BatchCalls, 4)
    assert.Equal(t, 5000, result.Success+result.Failed+result.Skipped)
}

func TestScanPublishesFailuresWithoutDroppingSuccessfulSignals(t *testing.T) {
    marketData.FailFor(id("SSE:600001"), errors.New("bad adjustment"))
    result := mustScan(t, service, []market.InstrumentID{id("SSE:600000"), id("SSE:600001")})
    assert.Equal(t, port.RunPartialSucceeded, result.Status)
    assert.Len(t, result.Failures, 1)
    assert.NotEmpty(t, result.Snapshot.Rows)
}
```

- [ ] **Step 4: Implement bounded parallel scan and incremental merge**

Use a fixed worker count from validated config. Load each timeframe/date range in bounded calls, group in the repository, create one fresh strategy instance per Instrument, and preserve errors by Instrument. For incremental runs, recompute dirty Instruments and merge with the previous immutable snapshot; strategy/config/data-factor changes force a full rebuild.

```go
func scanBatch(ctx context.Context, workers int, ids []market.InstrumentID, fn func(context.Context, market.InstrumentID) (port.SnapshotRow, bool, error)) scanBatchResult {
    jobs := make(chan market.InstrumentID)
    results := make(chan instrumentResult)
    var workersWG sync.WaitGroup
    workersWG.Add(workers)
    for i := 0; i < workers; i++ { go scanWorker(ctx, &workersWG, jobs, results, fn) }
    go func() { workersWG.Wait(); close(results) }()
    go produceInstruments(ctx, jobs, ids)
    return collectScanResults(ctx, results)
}
```

The producer stops on cancellation, channels are closed by their owner, and collection sorts rows by InstrumentID before persistence so repeated runs are byte-stable.

- [ ] **Step 5: Implement persisted retry classification**

Retry only `port.ErrTemporary`, timeout, and transient connection errors. A failed attempt calls `JobQueue.Retry` with delays of 250ms, 1s, and 4s based on the persisted attempt number; after the fourth claimed attempt, call `RunStore.Fail(ctx, run.ID, run.LeaseToken, failure)` instead. Parameter, strategy, data quality, and deterministic engine errors are terminal. Tests inject a fake clock and do not sleep, proving one initial execution plus three retries is the maximum even after lease reclamation.

```go
var retryDelays = []time.Duration{250 * time.Millisecond, time.Second, 4 * time.Second}

func retryAt(now time.Time, persistedAttempt int) (time.Time, bool) {
    if persistedAttempt < 1 || persistedAttempt >= 4 { return time.Time{}, false }
    return now.Add(retryDelays[persistedAttempt-1]), true
}
```

- [ ] **Step 6: Emit structured stage telemetry**

```go
defer telemetry.ObserveStage(ctx, port.StageObservation{
    RunID: run.ID, Stage: "feature_graph", StrategyID: run.StrategyID,
    StrategyVersion: run.StrategyVersion, DataVersion: run.DataVersion,
    Duration: clock.Since(start), Rows: int64(dataset.Len()),
})
```

Emit observations for queue wait, market load, dataset validation, FeatureGraph, strategy replay, engine execution, and persistence. `SlogTelemetry` logs only the declared fields; tests use a JSON `slog.Handler` and assert `run_id`, `stage`, strategy version, data version, duration, counts, and absence of request JSON or credentials.

- [ ] **Step 7: Run application tests and commit**

Run: `go test -race ./internal/application -cover`

Expected: PASS with at least 85% application coverage.

```bash
git add internal/application
git commit -m "feat: orchestrate durable backtests and scans"
```

---

### Task 14: V1 APIs and Existing-Path Adapters

**Files:**
- Create: `api/create_backtest_run.go`
- Create: `api/get_backtest_run.go`
- Create: `api/cancel_backtest_run.go`
- Create: `api/list_backtest_orders.go`
- Create: `api/list_backtest_trades.go`
- Create: `api/list_backtest_equity.go`
- Create: `api/create_scan_run.go`
- Create: `api/get_scan_run.go`
- Create: `api/cancel_scan_run.go`
- Create: `api/get_latest_signal_snapshot.go`
- Create: `api/strategy_handler.go`
- Test: matching `*_test.go` file for every handler above
- Modify: `api/get_stock_buy_signals.go`
- Modify: `api/get_stock_backtest.go`
- Modify: `api/handler.go`
- Modify: `api/router.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: application services from Task 13.
- Produces: durable `/api/v1/*` endpoints and compatible existing paths.

- [ ] **Step 1: Write failing create/status contract tests**

```go
func TestCreateBacktestRunReturnsAcceptedAndRunID(t *testing.T) {
    req := `{"instrument":"SSE:600000","strategy":"daily_b1_buy","strategy_version":"1","idempotency_key":"abc"}`
    w := performJSON(t, router, http.MethodPost, "/api/v1/backtest-runs", req)
    assert.Equal(t, http.StatusAccepted, w.Code)
    assertJSONPath(t, w.Body.Bytes(), "data.run_id", "run-1")
}

func TestInternalErrorIsRedacted(t *testing.T) {
    service.Err = errors.New("dial user:secret@db/table")
    w := perform(t, router, http.MethodGet, "/api/v1/backtest-runs/run-1")
    assert.NotContains(t, w.Body.String(), "secret")
}
```

- [ ] **Step 2: Implement v1 handlers and stable error mapping**

Map validation to 400, unknown strategy/run to 404, duplicate conflict to 409, snapshot-not-ready to 409, accepted jobs to 202, cancellation to 202, and unexpected errors to redacted 500. Require page sizes 1-1000 and stable sequence cursors.

```go
func writeApplicationError(c *gin.Context, op string, err error) {
    switch {
    case errors.Is(err, application.ErrInvalidRequest): respondCode(c, 400, "INVALID_REQUEST")
    case errors.Is(err, strategy.ErrUnknownStrategy), errors.Is(err, port.ErrRunNotFound): respondCode(c, 404, "NOT_FOUND")
    case errors.Is(err, port.ErrSnapshotNotReady): respondCode(c, 409, "SIGNAL_SNAPSHOT_NOT_READY")
    default: respondInternalError(c, op, err)
    }
}
```

Every create handler binds into an explicit request struct, calls `Validate`, passes `c.Request.Context()`, and returns `{code,message,data:{run_id,status}}`.

- [ ] **Step 3: Write failing compatibility tests**

```go
func TestLegacySignalEndpointReadsSnapshotWithoutStartingScan(t *testing.T) {
    w := perform(t, router, http.MethodGet, "/api/stocks/signal?strategy=daily_b1_buy")
    assert.Equal(t, http.StatusOK, w.Code)
    assert.Equal(t, 0, scanService.CreateCalls)
    assertJSONPath(t, w.Body.Bytes(), "data.codes.0", "600000")
}

func TestLegacyBacktestReturns202WhenSynchronousWaitExpires(t *testing.T) {
    fakeWaiter.Timeout = true
    w := perform(t, router, http.MethodGet, "/api/stocks/backtest?code=600000&strategy=daily_b1_buy")
    assert.Equal(t, http.StatusAccepted, w.Code)
    assertJSONPath(t, w.Body.Bytes(), "data.run_id", "run-1")
}
```

- [ ] **Step 4: Implement existing-path adapters and route registration**

The signal path strips exchange only in its existing `codes` response. The backtest path resolves the six-digit code through `t_instruments`; zero matches returns 404 and multiple exchange matches returns 409 instead of guessing. It creates a durable run, waits only for the configured synchronous timeout, and otherwise returns the Run ID.

```go
func (h *StockHandler) GetStockBuySignals(c *gin.Context) {
    snapshot, err := h.scanService.Latest(c.Request.Context(), legacySnapshotKey(c.Query("strategy")))
    if err != nil { writeApplicationError(c, "latest signal snapshot", err); return }
    respondSuccess(c, gin.H{"name": snapshot.Key.StrategyID, "codes": legacyCodes(snapshot.Rows)})
}

func (h *StockHandler) GetStockBacktest(c *gin.Context) {
    run, err := h.backtestService.Create(c.Request.Context(), legacyBacktestRequest(c))
    if err != nil { writeApplicationError(c, "create backtest", err); return }
    result, completed := h.backtestWaiter.Wait(c.Request.Context(), run.ID, h.syncWaitTimeout)
    if !completed { c.JSON(http.StatusAccepted, successResponse(runRef(run))); return }
    respondSuccess(c, result)
}
```

Extend dependency composition in the same task so `go test ./...` continues to compile. Keep the legacy services wired only until Task 15 removes their production call sites; no deployed release contains both engines.

- [ ] **Step 5: Run API tests and commit**

Run: `go test ./...`

Expected: PASS with one-file-per-handler convention preserved.

```bash
git add api main.go
git commit -m "feat: expose durable strategy run APIs"
```

---

### Task 15: Financial-Screen Relocation, Runtime Cutover, and Legacy Removal

**Files:**
- Create: `internal/financialscreen/filter.go`
- Create: `internal/financialscreen/profit_growth.go`
- Create: `internal/financialscreen/revenue_growth.go`
- Test: `internal/financialscreen/filter_test.go`
- Test: `internal/financialscreen/profit_growth_test.go`
- Test: `internal/financialscreen/revenue_growth_test.go`
- Modify: `business/signal_service.go`
- Modify: `business/signal_service_test.go`
- Delete: `business/stock_service.go`
- Delete: `business/stock_service_test.go`
- Delete: `business/stock_scheduler.go`
- Delete: `business/stock_scheduler_test.go`
- Modify: `business/query_service.go`
- Modify: `business/query_service_test.go`
- Modify: `api/save_stock_historical_data.go`
- Modify: `api/save_stock_historical_data_test.go`
- Modify: `api/append_stock_data.go`
- Modify: `api/append_stock_data_test.go`
- Modify: `api/get_stock_price.go`
- Modify: `api/get_stock_price_test.go`
- Modify: `main.go`
- Modify: `config/config.go`
- Modify: `data/data.go`
- Delete: `pkg/filter/*.go`
- Delete: `pkg/filter/financial/*.go`
- Delete: `pkg/strategy/*.go`

**Interfaces:**
- Consumes: all new runtime services and adapters.
- Produces: a single active technical strategy engine; unchanged financial-signal API semantics.

- [ ] **Step 1: Copy financial behavior into new characterization tests**

```go
func TestProfitGrowthMatchesLegacyCases(t *testing.T) {
    cases := legacyProfitGrowthCases()
    for _, tc := range cases {
        got := financialscreen.ProfitGrowth(tc.Reports, tc.Threshold, tc.QuarterCount)
        assert.Equal(t, tc.Want, got, tc.Name)
    }
}
```

Run: `go test ./internal/financialscreen`

Expected: FAIL because the relocated package is missing.

- [ ] **Step 2: Move financial-screen logic without changing formulas**

Implement the typed `ReportFilter` contract in `internal/financialscreen`, copy existing profit/revenue calculations, and run both new characterization tests and existing business tests before deleting old files.

```go
type ReportFilter interface {
    Match(reports []*model.FinancialReport) bool
}

func NewProfitGrowth(threshold float64, quarterCount int) (ReportFilter, error)
func NewRevenueGrowth(threshold float64, quarterCount int) (ReportFilter, error)
```

Validate finite thresholds and positive quarter counts. Preserve existing report ordering and missing-quarter behavior exactly as captured by characterization tests.

- [ ] **Step 3: Add new application dependencies and worker lifecycle configuration**

```go
type WorkerConfig struct {
    Count                int `yaml:"Count"`
    LeaseSeconds         int `yaml:"LeaseSeconds"`
    PollIntervalMillis   int `yaml:"PollIntervalMillis"`
    SyncWaitTimeoutSecs  int `yaml:"SyncWaitTimeoutSecs"`
    ScanBatchSize        int `yaml:"ScanBatchSize"`
}
```

`main.go` registers built-ins, constructs MySQL adapters and application services, starts bounded workers plus the new market scheduler with a cancellable root context, and gracefully stops them before closing the database. Historical-save, append, and price handlers use Task 11 services; the old StockDataService and stock scheduler are removed. `business.QueryService` retains only financial-report querying until that domain is migrated separately.

```go
rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
workers.Start(rootCtx)
serverErr := httpServer.Run(rootCtx)
workers.Wait()
return serverErr
```

- [ ] **Step 4: Switch business/API composition and delete legacy technical code**

Replace calls to `createStrategy`, `ScanAll`, `ScanLatest`, and per-code `FindRecentByCodes` with new application services. Move financial calls to `internal/financialscreen`. Confirm no production strategy import or application-layer per-code scan remains:

Run: `rg 'trading/pkg/(filter|strategy)|ScanLatest|ScanAll' --glob '*.go'; rg 'FindRecentByCodes' business api main.go internal/application`

Expected: no matches. Legacy repository methods may remain under `data/` solely for rollback tooling, but no runtime service calls them.

- [ ] **Step 5: Run affected tests and commit**

Run: `go test ./business ./api ./internal/...`

Expected: PASS.

```bash
git add internal/financialscreen business api/save_stock_historical_data.go api/save_stock_historical_data_test.go api/append_stock_data.go api/append_stock_data_test.go api/get_stock_price.go api/get_stock_price_test.go main.go config data/data.go pkg/filter pkg/strategy
git commit -m "refactor: cut over to the new strategy kernel"
```

---

### Task 16: Security, Documentation, Performance Gate, and Final Verification

**Files:**
- Modify: `Dockerfile`
- Modify: `.dockerignore`
- Modify: `config.example.yaml`
- Create: `README.md` if absent
- Modify: `api/api.md`
- Modify: `AGENTS.md`
- Modify: `model/README.md`
- Create: `internal/application/scan_fixture_test.go`
- Create: `internal/application/scan_benchmark_test.go`
- Create: `scripts/verify.sh`

**Interfaces:**
- Consumes: completed system.
- Produces: credential-safe image, current docs, reproducible benchmark and one-command verification.

- [ ] **Step 1: Write failing container-context security checks**

```bash
test "$(grep -c 'COPY config-nas.yaml' Dockerfile)" -eq 0
grep -q '^config-nas.yaml$' .dockerignore
grep -q '^\*.tar$' .dockerignore
```

Expected before the fix: the first and ignore checks fail.

- [ ] **Step 2: Remove baked configuration and harden ignore rules**

Delete `COPY config-nas.yaml /app/config.yaml`. Run the binary with `-config /app/config.yaml` only when the file is mounted, or allow `TRADING_CONFIG` to select the path. Ignore `config*.yaml` except `!config.example.yaml`, `*.tar`, database dumps, `.env*`, and local task directories. Do not open or print secret values.

```dockerfile
COPY --from=builder /out/trading /app/trading
USER app
ENTRYPOINT ["/app/trading"]
CMD ["-config", "/app/config.yaml"]
```

The documented container command mounts a read-only file: `-v "$PWD/config.yaml:/app/config.yaml:ro"`. Configuration loading reports only the path and redacts DB user, host, database name, and password from startup logs.

- [ ] **Step 3: Update all user-facing documentation**

Document:

- secure runtime config mounting;
- data migration dry-run and apply commands;
- strategy IDs/versions and fixed timing semantics;
- v1 run/status/cancel/result endpoints;
- legacy endpoint behavior;
- scan snapshot freshness and errors;
- MySQL-only first deployment and Redis/Kafka adoption triggers;
- known behavior change from future-data removal.

Update `model/README.md` at this final cutover point to require UTC `time.Time` for timestamps, a separate date-only trading value, explicit table names, and audit fields while preserving the one-table-per-file rule.

Remove stale scored-signal examples from `api/api.md`. Update `AGENTS.md` project structure and test commands.

- [ ] **Step 4: Add fixed 5000-instrument benchmark and assertions**

```go
func BenchmarkFullMarketScan5000(b *testing.B) {
    fixture := newScanFixture(5000, 140)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        if _, err := fixture.Service.Execute(context.Background(), fixture.RunID); err != nil {
            b.Fatal(err)
        }
    }
}
```

The integration performance test records stage durations, asserts bounded SQL count, requires at least 5x improvement against the checked-in baseline measurement, requires full scan under 8 seconds on the acceptance host, and snapshot P95 under 200ms.

- [ ] **Step 5: Create a deterministic verification script**

```bash
#!/usr/bin/env bash
set -euo pipefail
go test ./... -coverprofile=coverage.out
go test -race ./...
go vet ./...
go tool cover -func=coverage.out
go test -tags=integration ./internal/infrastructure/mysql -count=1
go test ./internal/application -run TestFullMarketScanPerformance -count=1
docker build --no-cache -t trading:verify .
```

The script calculates total coverage and exits nonzero below 80%; it also runs each of `./internal/market`, `./internal/indicator`, `./internal/strategy/...`, and `./internal/backtest` with a separate coverage profile and exits nonzero when any domain area is below 90%.

- [ ] **Step 6: Run full verification**

Run: `bash scripts/verify.sh`

Expected: all tests and vet pass, race detector passes, coverage gates pass, performance gates pass, and the image builds without local configuration.

- [ ] **Step 7: Inspect the final diff for secrets and unrelated files**

Run:

```bash
git status --short
git diff --check
git diff --name-only main...HEAD
git grep -nE '(password|passwd|secret|token)[[:space:]]*[:=][[:space:]]*[^$<{[:space:]]+' -- ':!go.sum' ':!docs/analysis'
```

Expected: user-owned dirty files remain unstaged, no whitespace errors, no credential values, and only migration-related files appear in the branch diff.

- [ ] **Step 8: Commit documentation and hardening**

```bash
git add Dockerfile .dockerignore config.example.yaml README.md api/api.md AGENTS.md model/README.md internal/application/scan_fixture_test.go internal/application/scan_benchmark_test.go scripts/verify.sh
git commit -m "docs: finalize strategy kernel migration"
```

- [ ] **Step 9: Run required final reviews**

Dispatch one fresh reviewer with `/simplify` to identify unnecessary abstractions and one fresh reviewer with `/code-review` to inspect correctness, regression risk, security, and test coverage. Apply only evidence-backed changes, rerun `bash scripts/verify.sh`, and commit each distinct correction separately.

- [ ] **Step 10: Integrate according to repository policy**

After every verification and review passes, fast-forward or merge the completed `codex/` branch into `main`, switch back to `main`, rerun the fast verification subset, and delete the merged development branch. Never overwrite the user's pre-existing dirty changes.
