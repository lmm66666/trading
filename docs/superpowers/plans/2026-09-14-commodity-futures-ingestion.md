# Commodity Futures Daily Ingestion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add versioned domestic commodity-futures daily bars, contract-specific fields, causal main-contract mappings, and raw/ratio-adjusted continuous series for AU, AG, FU, SC, LU, J, JM, and ZC.

**Architecture:** A fixed Python 3.12/AKShare sidecar converts exchange public data into a stable JSON contract; Go remains authoritative for identifiers, validation, persistence, main selection, continuous-series construction, scheduling, and queries. Existing version allocation and common market-bar tables are reused, while two futures-specific revision tables store settlements/open interest and main mappings.

**Tech Stack:** Go 1.25.7, Python 3.12, FastAPI, `akshare==1.18.94`, GORM, MySQL 5.7/8.0, Gin, Testify, pytest, Docker.

**Spec:** `docs/superpowers/specs/2026-09-13-market-data-ingestion-design.md`

## Global Constraints

- First-release exchanges/products are SHFE AU/AG/FU, INE SC/LU, DCE J/JM, and CZCE ZC; reject everything outside configured server-side allowlists.
- Store only daily data; do not add minute, tick, order-book, overseas, or execution functionality.
- Python may call only the fixed `get_futures_daily` mapping and must not accept function names, URLs, code, or file paths from requests.
- Sidecar uses Python 3.12 and exactly `akshare==1.18.94`; upgrades require fixture-contract updates.
- Main rule is `main_oi_hysteresis_v1`: 110% OI for two days, 125% for one day, minimum three-day hold, no backward delivery-month roll, next-observed-session effect.
- Continuous views are `RAW_MAIN` and `FORWARD_RATIO`; construction must satisfy prefix consistency and must never use a future roll anchor.
- Reuse `t_instruments`, `t_market_data_versions`, and `t_market_bars`; add only `t_futures_contract_daily` and `t_futures_main_mappings`.
- Database change is an offline maintenance migration; do not build dual write, shadow tables, online backfill, Redis, Kafka, or distributed leases.
- Use injected `slog`/Telemetry, parameterized SQL, explicit columns, bounded transactions, and indexes matching query predicates.
- Overall coverage remains above 80%; futures domain/main/continuous code remains above 90%.
- Preserve the user's staged `.claude/skills/stock` deletions and never include them in feature commits.

---

### Task 1: Extend Instrument Identity for Futures

**Files:**
- Modify: `internal/market/instrument.go`
- Modify: `internal/market/instrument_test.go`
- Modify: `internal/port/market_data.go`
- Modify: `internal/port/contracts_test.go`
- Modify: `internal/infrastructure/mysql/instrument_model.go`
- Modify: `internal/infrastructure/mysql/market_mapping.go`
- Modify: `internal/infrastructure/mysql/identity_schema_test.go`

**Interfaces:**
- Produces `AssetClass`, `InstrumentKind`, SHFE/INE/DCE/CZCE exchanges, real contract IDs such as `SHFE:AU202612`, and continuous IDs such as `SHFE:AU.MAIN`.

- [ ] **Step 1: Write failing identity tests**

Cover valid equity, futures contract and `.MAIN` identifiers; reject lowercase canonical IDs, invalid product, month 00/13, expired shorthand, exchange/product mismatch, and futures kinds on equity exchanges. Assert `ParseInstrumentID(id.String()) == id`.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/market ./internal/port -run 'Test.*(Instrument|Exchange|Scope)' -count=1`

Expected: FAIL for unsupported futures identities.

- [ ] **Step 3: Implement typed validation**

Keep `InstrumentID` as exactly `{Exchange, Code}` and derive asset class/kind from exchange and code; persist explicit classification columns in `InstrumentModel`. Keep equity six-digit validation unchanged. Validate futures codes with anchored parsing: `^[A-Z]{1,2}(20[0-9]{2})(0[1-9]|1[0-2])$` and `^[A-Z]{1,2}\.MAIN$`, then enforce the product allowlist by exchange.

```go
func (id InstrumentID) AssetClass() AssetClass
func (id InstrumentID) Kind() InstrumentKind
func (id InstrumentID) Product() string
func (id InstrumentID) DeliveryMonth() (time.Time, bool)
```

- [ ] **Step 4: Update persistence mapping and schema expectations**

Expand exchange to 16 and code to 32, add `asset_class`, `instrument_kind`, `product_code`, nullable `delivery_month`, `last_trade_date`, `contract_multiplier`, and `tick_size`. Map existing equities to `EQUITY`/`SPOT_EQUITY` without changing their IDs.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/market ./internal/port ./internal/infrastructure/mysql -count=1`

```bash
git add internal/market/instrument.go internal/market/instrument_test.go internal/port/market_data.go internal/port/contracts_test.go internal/infrastructure/mysql/instrument_model.go internal/infrastructure/mysql/market_mapping.go internal/infrastructure/mysql/identity_schema_test.go
git commit -m "feat: add futures instrument identities"
```

### Task 2: Define Futures Daily and Source Contracts

**Files:**
- Create: `internal/market/futures_daily.go`
- Create: `internal/market/futures_daily_test.go`
- Create: `internal/port/futures_source.go`
- Create: `internal/port/futures_data.go`
- Modify: `internal/port/contracts_test.go`

**Interfaces:**
- Produces:

```go
type FuturesDaily struct {
    Bar Bar
    Instrument InstrumentID
    TradeDate time.Time
    PreSettlement Price
    Settlement Price
    VolumeLots int64
    OpenInterestLots int64
    Turnover Money
    RawSymbol string
    Product string
    StatisticsBasis string
    ProviderVersion string
}

type FuturesPartition struct {
    Exchange market.Exchange
    TradeDate time.Time
    Rows []market.FuturesDaily
    Provider string
    ProviderVersion string
}

type FuturesSource interface {
    FetchDaily(context.Context, market.Exchange, time.Time, []string) (FuturesPartition, error)
}

type FuturesDataWriter interface {
    PublishContract(context.Context, FuturesContractBatch) (market.DataVersion, error)
    PublishContinuous(context.Context, FuturesContinuousBatch) (market.DataVersion, error)
}

type FuturesContractBatch struct {
    Source string
    Instrument market.InstrumentID
    Daily []market.FuturesDaily
}

type FuturesContinuousBatch struct {
    Source string
    Instrument market.InstrumentID
    Bars []market.Bar
    Mappings []market.MainMapping
    DependsThrough market.DataVersion
}
```

- [ ] **Step 1: Write failing value and canonicalization tests**

Cover positive settlement values, non-negative volume/OI/turnover, exact trade-date UTC, raw-symbol length/case, provider metadata, duplicates, sorted canonical rows, and rejection of exchange aggregate codes `0`, `88`, `888`, `99`.

- [ ] **Step 2: Run RED, implement minimal values/ports, run GREEN**

Run RED then GREEN: `go test ./internal/market ./internal/port -run TestFutures -count=1`

- [ ] **Step 3: Commit**

```bash
git add internal/market/futures_daily.go internal/market/futures_daily_test.go internal/port/futures_source.go internal/port/futures_data.go internal/port/contracts_test.go
git commit -m "feat: define futures daily contracts"
```

### Task 3: Build the Fixed AKShare Sidecar

**Files:**
- Create: `sidecar/akshare-futures/pyproject.toml`
- Create: `sidecar/akshare-futures/app/main.py`
- Create: `sidecar/akshare-futures/app/provider.py`
- Create: `sidecar/akshare-futures/app/schema.py`
- Create: `sidecar/akshare-futures/tests/test_api.py`
- Create: `sidecar/akshare-futures/tests/test_provider.py`
- Create: `sidecar/akshare-futures/tests/fixtures/*.json`
- Create: `sidecar/akshare-futures/Dockerfile`

**Interfaces:**
- Consumes only `POST /v1/futures/daily` with request ID, one exchange/date, and allowlisted products.
- Produces schema version 1 response and stable errors `INVALID_ARGUMENT`, `UPSTREAM_UNEXPECTED_EMPTY`, `UPSTREAM_BAD_PAYLOAD`, `UPSTREAM_TIMEOUT`.

- [ ] **Step 1: Freeze complete exchange fixtures and write failing provider tests**

Create one sanitized full-column fixture for SHFE, INE, DCE, and CZCE. Assert literal normalized rows for AU, SC, J, and ZC, including decimal strings and raw symbols. Cover holiday empty, unexpected empty DataFrame, HTML/schema drift, duplicate symbols, invalid numeric cells, and timeout.

- [ ] **Step 2: Run pytest and verify RED**

Run: `cd sidecar/akshare-futures && python3.12 -m pytest -q`

Expected: FAIL because the app does not exist.

- [ ] **Step 3: Implement the provider allowlist**

Map each supported exchange to the fixed AKShare daily function call. Normalize columns through explicit exchange-specific maps; never dispatch with `getattr` from request data. Return decimal strings exactly and classify known holiday calendars separately from unexpected empties.

```python
PROVIDERS = {
    "SHFE": lambda day: ak.get_futures_daily(start_date=day, end_date=day, market="SHFE"),
    "INE": lambda day: ak.get_futures_daily(start_date=day, end_date=day, market="INE"),
    "DCE": lambda day: ak.get_futures_daily(start_date=day, end_date=day, market="DCE"),
    "CZCE": lambda day: ak.get_futures_daily(start_date=day, end_date=day, market="CZCE"),
}
provider = PROVIDERS[request.exchange]
```

- [ ] **Step 4: Implement HTTP schema and safe errors**

Use strict Pydantic models with `extra="forbid"`, a 1 MiB request limit, and response bodies containing only request ID, stable code, retryable flag, and safe message. Never expose stack traces or upstream HTML.

- [ ] **Step 5: Run tests, build image, and commit**

Run:

```bash
cd sidecar/akshare-futures && python3.12 -m pytest -q
docker build -t trading-akshare-futures:test .
```

```bash
git add sidecar/akshare-futures
git commit -m "feat: add fixed akshare futures sidecar"
```

### Task 4: Implement the Go Sidecar Adapter

**Files:**
- Create: `pkg/broker/akshare_futures.go`
- Create: `pkg/broker/akshare_futures_test.go`

**Interfaces:**
- Implements `port.FuturesSource.FetchDaily` and accepts one configured base URL plus fixed timeout.

- [ ] **Step 1: Write failing `httptest` contract tests**

Assert exact schema version, exchange/date/products, request ID propagation, decimal fixed-point conversion, response-size limit, unknown fields, duplicate rows, wrong provider version, 400 mapping, retryable 502/504 mapping, cancellation, and no response-body leakage in errors.

- [ ] **Step 2: Run RED**

Run: `go test ./pkg/broker -run TestAkshareFutures -count=1`

- [ ] **Step 3: Implement strict adapter and run GREEN**

Use `http.NewRequestWithContext`, `DisallowUnknownFields`, one JSON value, a 4 MiB response cap, explicit 15-second timeout, and decimal-string fixed-point parsers. Do not retry inside the adapter.

- [ ] **Step 4: Commit**

```bash
git add pkg/broker/akshare_futures.go pkg/broker/akshare_futures_test.go
git commit -m "feat: add akshare futures adapter"
```

### Task 5: Persist Contract Daily Fields Atomically

**Files:**
- Create: `internal/infrastructure/mysql/futures_daily_model.go`
- Create: `internal/infrastructure/mysql/futures_main_mapping_model.go`
- Create: `internal/infrastructure/mysql/futures_repository.go`
- Create: `internal/infrastructure/mysql/futures_repository_test.go`
- Create: `internal/infrastructure/mysql/futures_repository_integration_test.go`
- Modify: `internal/infrastructure/mysql/migrate.go`
- Modify: `internal/infrastructure/mysql/migrate_integration_test.go`

**Interfaces:**
- Implements `FuturesDataWriter.PublishContract` in one transaction sharing the existing version allocator and market-bar revision helpers.

- [ ] **Step 1: Write failing SQL behavior tests**

Assert explicit-column reads, parameterized predicates, `PENDING` version insertion, bar revisions, futures-field revisions, `COMPLETE` transition, rollback on any middle failure, no-op digest behavior, and indexes matching instrument/date/version lookups.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/infrastructure/mysql -run TestFuturesRepository -count=1`

- [ ] **Step 3: Implement models and repository**

Use `valid_from_version`/nullable `valid_to_version` on both new tables. Close current revisions and insert replacements inside one bounded transaction. Never publish a complete version before both common and futures-specific rows succeed.

- [ ] **Step 4: Add MySQL 5.7/8.0 integration coverage**

Verify migration, unique/index definitions, rollback, fixed-version reads, and same-digest no-op against real containers through the existing `dbtest` harness.

- [ ] **Step 5: Run tests and commit**

```bash
go test ./internal/infrastructure/mysql -count=1
git add internal/infrastructure/mysql/futures_* internal/infrastructure/mysql/migrate.go internal/infrastructure/mysql/migrate_integration_test.go
git commit -m "feat: persist versioned futures daily data"
```

### Task 6: Implement Causal Main-Contract Selection

**Files:**
- Create: `internal/market/futures_main.go`
- Create: `internal/market/futures_main_test.go`

**Interfaces:**
- Produces:

```go
type FuturesSession struct {
    TradeDate time.Time
    Contracts []FuturesDaily
}

type MainPolicy struct {
    RuleVersion string
    NormalThresholdBPS int64
    FastThresholdBPS int64
    ConfirmSessions int
    MinimumHoldSessions int
    ForceBeforeLastTradeSessions int
}

type MainMapping struct {
    Continuous InstrumentID
    TradeDate time.Time
    Contract InstrumentID
    DecisionDate time.Time
    Reason MainChangeReason
    OldOpenInterest int64
    NewOpenInterest int64
    OldVolume int64
    NewVolume int64
}

func BuildMainMappings(product InstrumentID, sessions []FuturesSession, policy MainPolicy) ([]MainMapping, error)
```

- [ ] **Step 1: Write failing table tests for every rule**

Use literal OI/volume sessions for initial selection, 110% two-day switch, 125% fast switch, three-day minimum hold, no backward roll, last-trade forced roll, delivery-month fallback, two-day no-trade roll, zero-OI roll, deterministic ties, and no eligible/liquid candidate. For every scenario assert the mapping becomes effective only on the next observed session.

- [ ] **Step 2: Add prefix/suffix poisoning tests**

For every prefix `k`, assert `Build(all)[:k] == Build(all[:k])`; mutate dates after `k` to extreme OI and assert earlier mappings do not change.

- [ ] **Step 3: Run RED, implement state machine, run GREEN**

Run: `go test ./internal/market -run TestBuildMainMappings -count=1`

Implement a forward-only state machine with explicit pending candidate streak, hold count, and forced reason. Do not search future sessions for confirmation or execution dates.

- [ ] **Step 4: Commit**

```bash
git add internal/market/futures_main.go internal/market/futures_main_test.go
git commit -m "feat: select causal futures main contracts"
```

### Task 7: Build Raw and Forward-Ratio Continuous Series

**Files:**
- Create: `internal/market/futures_continuous.go`
- Create: `internal/market/futures_continuous_test.go`

**Interfaces:**
- Produces:

```go
type ContinuousView string
const (
    RawMain ContinuousView = "RAW_MAIN"
    ForwardRatio ContinuousView = "FORWARD_RATIO"
)

type ContinuousFactor struct {
    EffectiveTime time.Time
    Numerator int64
    Denominator int64
}

func BuildContinuous(id InstrumentID, mappings []MainMapping, contracts map[InstrumentID][]Bar, view ContinuousView) ([]Bar, []ContinuousFactor, error)
```

- [ ] **Step 1: Write failing literal price tests**

Assert `RAW_MAIN` preserves switch gaps. For `FORWARD_RATIO`, hand-calculate `ratio = newClose/oldClose`, `newFactor = oldFactor/ratio`, apply one factor to OHLC/settlement only, and leave volume/OI unchanged. Cover missing same-day anchors, zero prices, duplicate mapping dates, and factor overflow.

- [ ] **Step 2: Add prefix consistency tests**

Assert every output prefix is unchanged by later mappings/bars and that a missing future anchor cannot poison earlier output.

- [ ] **Step 3: Run RED, implement, run GREEN, commit**

Run: `go test ./internal/market -run TestBuildContinuous -count=1`

```bash
git add internal/market/futures_continuous.go internal/market/futures_continuous_test.go
git commit -m "feat: build futures continuous series"
```

### Task 8: Add Futures Ingestion, Scheduling, and Backfill CLI

**Files:**
- Create: `internal/application/futures_ingestion_service.go`
- Create: `internal/application/futures_ingestion_service_test.go`
- Create: `internal/application/futures_scheduler.go`
- Create: `internal/application/futures_scheduler_test.go`
- Create: `cmd/futures-sync/main.go`
- Create: `cmd/futures-sync/main_test.go`
- Modify: `config/config.go`
- Modify: `config.example.yaml`

**Interfaces:**
- Produces per-exchange/date ingestion, recent-ten-session refresh, three-attempt jittered retry, partial-failure summary, and `futures-sync -from YYYY-MM-DD -to YYYY-MM-DD`.

- [ ] **Step 1: Write failing ingestion tests**

Assert partition exchange/date consistency, allowlist filtering before the source call, contract-level atomic publishes, no main mapping when one candidate contract fails, and old complete versions surviving errors.

- [ ] **Step 2: Write failing scheduler/CLI tests**

Use a fake clock and deterministic jitter source. Assert bounded concurrency, three attempts only for retryable errors, cancellation, exchange ordering, ten-session overlap, non-zero CLI exit on failure, and idempotent rerun from an explicit date.

- [ ] **Step 3: Run RED, implement services, run GREEN**

Run: `go test ./internal/application ./cmd/futures-sync -run TestFutures -count=1`

- [ ] **Step 4: Commit**

```bash
git add internal/application/futures_* cmd/futures-sync config/config.go config.example.yaml
git commit -m "feat: ingest and schedule futures data"
```

### Task 9: Add Version-Pinned Futures Queries and API

**Files:**
- Create: `internal/application/futures_query_service.go`
- Create: `internal/application/futures_query_service_test.go`
- Create: `api/get_futures_bars.go`
- Create: `api/get_futures_bars_test.go`
- Create: `api/get_futures_main_mappings.go`
- Create: `api/get_futures_main_mappings_test.go`
- Modify: `api/kernel_handler.go`
- Modify: `api/router.go`
- Modify: `api/api.md`

**Interfaces:**
- Produces `GET /api/v1/futures/bars` and `GET /api/v1/futures/main-mappings` with instrument, date range, view, optional version, and stable errors.

- [ ] **Step 1: Write failing query tests**

Assert omitted version resolves latest exactly once, all dependent reads use that version, real contract/RAW_MAIN/FORWARD_RATIO routing, date bounds, and missing roll-factor errors.

- [ ] **Step 2: Write failing Gin handler tests**

Cover strict query cardinality, invalid instrument/view/date/version, 404 not found, 409 unavailable continuous factor, and exact response fields without internal database IDs.

- [ ] **Step 3: Run RED, implement, run GREEN**

Run: `go test ./internal/application ./api -run 'Test.*Futures' -count=1`

- [ ] **Step 4: Document and commit**

```bash
git add internal/application/futures_query_service* api/get_futures_* api/kernel_handler.go api/router.go api/api.md
git commit -m "feat: expose futures daily queries"
```

### Task 10: Wire Runtime, Offline Migration, and Full Verification

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `internal/application/telemetry.go`
- Modify: `internal/application/telemetry_test.go`
- Create: `cmd/migrate-futures-schema/main.go`
- Create: `cmd/migrate-futures-schema/main_test.go`
- Modify: `AGENTS.md`
- Modify: `Dockerfile`
- Modify: `scripts/verify.sh`

**Interfaces:**
- Wires the configured sidecar, repository, scheduler, query services, graceful shutdown, and an explicit offline schema migration command.

- [ ] **Step 1: Write failing composition and migration tests**

Assert disabled futures configuration makes no sidecar calls, enabled configuration requires a valid fixed base URL and allowlists, shutdown joins the scheduler before DB close, and offline migration performs preflight duplicate/length checks before explicit ALTER statements.

- [ ] **Step 2: Run RED, implement runtime and migration, run GREEN**

Run: `go test . ./cmd/migrate-futures-schema ./internal/application -run 'Test.*Futures' -count=1`

- [ ] **Step 3: Add structured telemetry and docs**

Emit one record per partition attempt and one summary per run with allowlisted fields: component, operation, exchange, trade_date, request_id, attempt, duration_ms, rows, status, error_code. Update `AGENTS.md` with sidecar startup, CLI backfill, offline migration, and schedule commands.

- [ ] **Step 4: Run coverage gates**

```bash
go test ./internal/market ./internal/application ./pkg/broker -coverprofile=/tmp/futures.cover
go tool cover -func=/tmp/futures.cover
cd sidecar/akshare-futures && python3.12 -m pytest --cov=app --cov-report=term-missing
```

Expected: futures domain/main/continuous above 90%; project trajectory above 80%.

- [ ] **Step 5: Run full repository verification**

Run: `bash scripts/verify.sh`

Expected: unit, coverage, race, vet, performance, MySQL 5.7/8.0, both image builds, and secret/config exclusion gates pass. Report Docker-only gates explicitly if the daemon is unavailable.

- [ ] **Step 6: Commit**

```bash
git add main.go main_test.go internal/application/telemetry.go internal/application/telemetry_test.go cmd/migrate-futures-schema AGENTS.md Dockerfile scripts/verify.sh
git commit -m "feat: wire commodity futures ingestion"
```
