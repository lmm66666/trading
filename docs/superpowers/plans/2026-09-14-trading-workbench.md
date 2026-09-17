# A 股图表工作台 Implementation Plan

> 历史计划（已废止）：MySQL 版本和验收指令已由 [REQ-2026-002](../../changes/archive/legacy/REQ-2026-002-mysql8-cross-architecture.md)与[MySQL 设计](../../design/internal/infrastructure/mysql.md)取代，本文仅保留需求追溯价值。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付一个可搜索 A 股并展示版本固定的日/周 K 线、成交量和技术指标的响应式 Web 工作台。

**Architecture:** React/Vite 前端只负责交互与绘制；Go application service 负责证券搜索、行情版本固定、历史裁剪和权威指标计算。生产环境由 Go 同源托管 Vite 静态产物，开发环境使用 Vite proxy。

**Tech Stack:** Go 1.25.7、Gin、GORM/MySQL 8.4.x LTS、React、TypeScript、Vite、Vitest、Testing Library、Playwright、Lightweight Charts 5.2。

**Spec:** `docs/superpowers/specs/2026-09-14-trading-workbench-design.md`

## Global Constraints

- 新计算只读取大于 0 的 COMPLETE 行情版本，连续图表分页不得混合版本。
- Price/Money 在领域和数据库中继续使用缩放 10000 的有符号整数。
- 依赖方向保持 `api/infrastructure -> application -> port/domain`。
- 第一版只支持 DAY/WEEK、RAW/QFQ、SMA/EMA/MACD/KDJ，最多 16 个指标。
- 不修改旧 `/api/stocks/price` 行为，不引入 Redis、Kafka、Elasticsearch、Redux 或用户账户。
- 所有新逻辑测试先行；总覆盖率超过 80%，核心 Go 领域包超过 90%。

---

### Task 1: 证券目录查询

**Files:**
- Create: `internal/port/instrument_catalog.go`
- Create: `internal/application/instrument_query_service.go`
- Create: `internal/application/instrument_query_service_test.go`
- Create: `api/search_instruments.go`
- Create: `api/search_instruments_test.go`
- Modify: `internal/infrastructure/mysql/api_readers.go`
- Modify: `internal/infrastructure/mysql/api_readers_test.go`
- Modify: `api/kernel_handler.go`
- Modify: `api/router.go`
- Modify: `main.go`

**Interfaces:**
- Produces: `port.InstrumentCatalog.Search(context.Context, port.InstrumentSearch) ([]port.InstrumentSummary, error)` and `Get(context.Context, market.InstrumentID) (port.InstrumentSummary, error)`.
- Produces: `application.InstrumentQueryService.Search(context.Context, application.InstrumentSearchQuery)` and `Get` for the API and chart service.

- [ ] **Step 1: Write failing port/application tests**

```go
func TestInstrumentSearchValidatesAndDelegates(t *testing.T) {
    catalog := &catalogStub{items: []port.InstrumentSummary{{ID: market.InstrumentID{Exchange: market.SZSE, Code: "002415"}, Name: "海康威视", Active: true}}}
    service := application.NewInstrumentQueryService(catalog)
    got, err := service.Search(context.Background(), application.InstrumentSearchQuery{Query: "海康", Limit: 20})
    require.NoError(t, err)
    require.Equal(t, "海康威视", got[0].Name)
}
```

- [ ] **Step 2: Run the focused tests and confirm RED**

Run: `go test ./internal/application ./internal/port`

Expected: compilation fails because catalog types do not exist.

- [ ] **Step 3: Implement the bounded catalog contracts and application validation**

Implement byte limits, supported exchanges, active-only semantics and default/max limit 20/50. Keep SQL and GORM out of application.

- [ ] **Step 4: Write failing MySQL ordering tests**

Cover exact code before prefix, code results before name results, stable exchange/code ordering, empty results and database errors. Require parameterized predicates and an explicit `Limit`.

- [ ] **Step 5: Implement repository search/get**

Use GORM expressions with bound arguments. The table contains about 5000 rows, so bounded `LIKE` name search is acceptable in phase one; do not add a search service.

- [ ] **Step 6: Write failing handler tests**

Cover `q`, optional `exchange`, duplicate/invalid parameters, limit bounds, empty items and normalized DTO fields for `GET /api/v1/instruments`.

- [ ] **Step 7: Add the route and dependency wiring**

Inject the application service through `KernelServices` and register `GET /api/v1/instruments` only when the kernel is configured.

- [ ] **Step 8: Run tests and commit**

Run: `go test ./internal/port ./internal/application ./internal/infrastructure/mysql ./api`

Commit: `feat: add instrument catalog queries`

### Task 2: 版本固定的图表查询

**Files:**
- Create: `internal/application/chart_query_service.go`
- Create: `internal/application/chart_query_service_test.go`
- Create: `api/query_chart.go`
- Create: `api/query_chart_test.go`
- Modify: `api/kernel_handler.go`
- Modify: `api/router.go`
- Modify: `main.go`
- Modify: `api/api.md`

**Interfaces:**
- Consumes: `port.MarketData`, `port.InstrumentCatalog`, `indicator.Build`.
- Produces: `ChartQueryService.Query(context.Context, ChartQuery) (ChartResult, error)`.
- Produces: `POST /api/v1/chart-queries` with strict JSON and the response defined by the spec.

- [ ] **Step 1: Write failing validation tests**

```go
func TestChartQueryRejectsUnsupportedIndicator(t *testing.T) {
    service := application.NewChartQueryService(marketStub{}, catalogStub{})
    _, err := service.Query(context.Background(), application.ChartQuery{
        Instrument: testID, Timeframe: market.Day, View: market.ForwardAdjusted,
        Limit: 400, Indicators: []application.IndicatorRequest{{Kind: "RSI", Period: 14}},
    })
    require.ErrorIs(t, err, application.ErrInvalidRequest)
}
```

Cover DAY/WEEK, RAW/QFQ, 100–1000 limit, at most 16 unique indicator requests, SMA/EMA period 1–500, MACD ordered positive parameters and KDJ period 1–500.

- [ ] **Step 2: Run the focused test and confirm RED**

Run: `go test ./internal/application -run ChartQuery`

Expected: compilation fails because chart query types do not exist.

- [ ] **Step 3: Implement query validation and version resolution**

Resolve version 0 once. Validate the catalog entry. Treat `before` as exclusive by subtracting one microsecond from the query end. Read no more than the existing 20-year bound.

- [ ] **Step 4: Write failing behavior tests for paging and indicators**

Use real `market.Dataset` and `indicator.Build`; assert ascending Bar output, `has_more`, `next_before`, version reuse, QFQ values, omitted invalid SMA points, and MACD/KDJ component names.

- [ ] **Step 5: Implement full-history compute then response trimming**

Convert Bar values only at the application DTO boundary. Expand MACD/KDJ requests to their component refs, build once per unique ref, then trim points to returned Bar times. Preserve deterministic series ordering.

- [ ] **Step 6: Write failing strict handler tests**

Cover complete request mapping, default limit, RFC3339 UTC normalization, unknown fields, a second JSON value, malformed instrument, stable 400/404 responses and serialized `data_version`/cursor.

- [ ] **Step 7: Add handler, route, wiring and API documentation**

Register the new service in `KernelServices`; keep the legacy price endpoint untouched.

- [ ] **Step 8: Run tests and commit**

Run: `go test ./internal/application ./api`

Commit: `feat: add versioned chart queries`

### Task 3: React 图表工作台

**Files:**
- Create: `web/package.json`
- Create: `web/package-lock.json`
- Create: `web/tsconfig.json`
- Create: `web/vite.config.ts`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/app/App.tsx`
- Create: `web/src/api/client.ts`
- Create: `web/src/features/instruments/InstrumentSearch.tsx`
- Create: `web/src/features/instruments/InstrumentSearch.test.tsx`
- Create: `web/src/features/chart/ChartWorkspace.tsx`
- Create: `web/src/features/chart/ChartWorkspace.test.tsx`
- Create: `web/src/features/chart/FinancialChart.tsx`
- Create: `web/src/features/chart/chartData.ts`
- Create: `web/src/features/chart/chartData.test.ts`
- Create: `web/src/features/chart/IndicatorManager.tsx`
- Create: `web/src/styles.css`
- Create: `web/src/test/setup.ts`

**Interfaces:**
- Consumes: `GET /api/v1/instruments` and `POST /api/v1/chart-queries`.
- Produces: URL state `symbol`, `timeframe`, `view`; localStorage key `trading.chart.indicators.v1`.
- Produces: a chart adapter that maps semantic API series to Lightweight Charts panes without leaking chart APIs into the page.

- [ ] **Step 1: Scaffold test tooling and write failing pure-data tests**

Test response parsing, duplicate-free prepend by `close_time`, default MA requests and URL state normalization. Run `npm test -- --run` and confirm RED because helpers do not exist.

- [ ] **Step 2: Implement API types and pure chart state helpers**

Use a small typed `fetchJSON` wrapper. Reject `{code != 0}` and malformed essential fields with a user-readable error. Keep server numeric values as numbers because chart price/amount DTOs are already decimal API values.

- [ ] **Step 3: Write failing instrument-search component tests**

Use fake timers and a stubbed API. Assert 200ms debounce, arrow navigation, Enter selection, Escape clearing, loading, empty and retryable error states.

- [ ] **Step 4: Implement the two-column shell and search**

Use semantic input/listbox/option roles and visible focus rings. Keep recent selections local; do not implement cloud watchlists.

- [ ] **Step 5: Write failing workspace/indicator tests**

Assert initial SMA 5/20/60 request, DAY/QFQ defaults, URL changes, MACD/KDJ pane creation, indicator removal, retry and request version pinning during load-more.

- [ ] **Step 6: Implement ChartWorkspace and IndicatorManager**

Limit separate panes to three. Persist indicator configuration. Show instrument name/code, O/H/L/C, change, data date and data version. Use skeletons after 300ms and explicit empty/error recovery actions.

- [ ] **Step 7: Implement the Lightweight Charts adapter**

Create candle, volume and line/histogram series; synchronize panes through the library's native pane API. Use red-up/green-down colors, semantic CSS tokens, ResizeObserver cleanup and TradingView attribution.

- [ ] **Step 8: Add responsive and accessible behavior**

At widths below 768px, open search as a dismissible drawer. All controls have 44px hit areas, keyboard labels and reduced-motion behavior. Provide a collapsible recent-Bar data table and text chart summary.

- [ ] **Step 9: Run frontend checks and commit**

Run: `npm test -- --run && npm run build`

Commit: `feat: build trading chart workbench`

### Task 4: 同源托管、端到端验证与文档

**Files:**
- Create: `api/serve_web.go`
- Create: `api/serve_web_test.go`
- Create: `web/playwright.config.ts`
- Create: `web/e2e/workbench.spec.ts`
- Modify: `api/router.go`
- Modify: `main.go`
- Modify: `Dockerfile`
- Modify: `.dockerignore`
- Modify: `scripts/verify.sh`
- Modify: `AGENTS.md`
- Modify: `api/api.md`

**Interfaces:**
- Consumes: `web/dist` built in the Docker Node stage.
- Produces: `/`, `/assets/*` and SPA fallback without intercepting `/api/*`.

- [ ] **Step 1: Write failing web-serving tests**

Create a temporary dist directory with `index.html` and an asset. Assert root/asset success, client-route fallback and unchanged JSON 404 behavior under `/api`.

- [ ] **Step 2: Implement optional static serving**

Static files are enabled only when the configured directory contains `index.html`; local API-only startup remains valid. Prevent path traversal and do not use a catch-all that masks API errors.

- [ ] **Step 3: Add frontend build to Docker and verification**

Use a pinned Node LTS builder with `npm ci`, `npm test -- --run` and `npm run build`; copy only `dist` into the non-root runtime image. Ensure `.dockerignore` does not exclude `web/` sources required by the build.

- [ ] **Step 4: Add the end-to-end happy path**

Mock only network responses at the browser boundary. Exercise search, select, default chart request, timeframe/view change, add MACD, remove it and trigger load-more. Do not assert canvas pixels; assert visible state and outgoing contracts.

- [ ] **Step 5: Update project documentation**

Document `cd web && npm ci && npm run dev`, production static behavior, new API examples, end-of-day limitation and Lightweight Charts attribution requirement in `AGENTS.md` and `api/api.md`.

- [ ] **Step 6: Run complete verification**

Run:

```bash
go test ./...
go vet ./...
cd web && npm test -- --run && npm run build
cd .. && bash scripts/verify.sh
docker buildx build --platform linux/amd64 -t trading:workbench --load .
```

Confirm Go and frontend coverage exceed 80%; confirm core Go package gates remain above 90%. Report Docker/MySQL limitations explicitly if the daemon is unavailable.

- [ ] **Step 7: Commit**

Commit: `build: serve and verify the web workbench`

### Task 5: Review and integration

**Files:** all files changed by Tasks 1–4.

**Interfaces:** no new interfaces; this task verifies the approved spec and project constraints.

- [ ] **Step 1: Run a sub-agent `/code-review`**

Request review of the branch diff against `main`, focusing on API version consistency, MySQL query bounds, indicator stability, React lifecycle leaks, accessibility and test gaps.

- [ ] **Step 2: Reproduce every accepted finding with a failing test**

For each valid defect, add the smallest test that fails for the reported behavior before changing production code.

- [ ] **Step 3: Fix findings and rerun complete verification**

Keep fixes scoped to reviewed defects. Repeat Task 4 Step 6.

- [ ] **Step 4: Merge and clean up**

From the primary checkout, merge `codex/trading-workbench` into `main` with a non-fast-forward merge, verify the main checkout, remove the worktree, and delete the merged branch. Preserve pre-existing uncommitted user changes in the primary checkout.
