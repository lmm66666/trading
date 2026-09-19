---
id: CHG-2026-09-20-strategy-frontend-DESIGN
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-20"
approved_revision: "c69c9fa:docs/changes/active/2026-09-20-strategy-frontend/design.md"
approved_scope: [SWF-001, SWF-002, SWF-003, SWF-004, SWF-005, SWF-006, SWF-007, SWF-008, SWF-009, SWF-010]
---

# 目标设计：前端策略功能（扫描与回测）

> 批准证据：用户于 2026-09-20 会话通过评审界面批准本设计全量，按第 5 节顺序实施，测试先行。

## 1. 合并后状态

### 1.1 视图路由与上下文（App）

`App` 新增 `view` 状态：`'chart' | 'scan' | 'backtest'`，由 `readWorkbenchState` 扩展读取 URL 参数 `tab`（白名单三个值，非法或缺省回落 `'chart'`），`writeWorkbenchState` 同步写回；`workspace-column` 内按 `view` 渲染 `ChartWorkspace` / `ScanPanel` / `BacktestPanel`，侧栏搜索与移动端搜索行为不变。

顶层持有并经 props 下传的共享上下文：

- `selectedSymbol`：现有状态；`BacktestPanel` 以它作为证券默认值，扫描结果行点击调用现有 `selectInstrument` 并把 `view` 切回 `'chart'`。
- 当前任务标识：`scanRunId` / `backtestRunId` 各一个，写入 localStorage（`wb.scan_run_id`、`wb.backtest_run_id`），初始化时读回；面板卸载不丢失，重开可恢复（SWF-009）。同一时刻每类只保留最近一个任务。

### 1.2 API 客户端（api/client.ts 扩展）

沿用现有 `request<T>` Envelope 解析，新增 DTO 与函数：

```ts
interface StrategyParam { name: string; default: number; min: number; max: number; integer: boolean }
interface StrategyDefinition {
  strategy: string; version: string; primary_timeframe: string
  warmup_bars: number; default_hold_bars: number; parameters: StrategyParam[]
}
listStrategies(): Promise<StrategyDefinition[]>              // GET /api/v1/strategies
type RunKind = 'scan' | 'backtest'
getRun(kind, runId): Promise<RunStatus>                       // GET /api/v1/{scan,backtest}-runs/:id
cancelRun(kind, runId): Promise<void>                         // POST .../:id/cancel，空对象体
createScanRun(input: ScanRunInput): Promise<{run_id, status}> // POST /api/v1/scan-runs
createBacktestRun(input: BacktestRunInput): Promise<{run_id, status}> // POST /api/v1/backtest-runs
fetchSnapshotPage(params): Promise<SnapshotPage>              // GET /api/v1/signal-snapshots/latest
fetchRunPage<T>(kind, runId, resource, after?, limit?): Promise<Page<T>> // orders/trades/equity 共用
```

`RunStatus` 含 `run_id,status,kind,strategy,strategy_version,data_version,engine_version,attempts,cancel_requested_at`；`SnapshotPage` 含 `snapshot_id,run_id,key,data_version,rows,failures,next_sequence?`；`Page<T>` 含 `items,next_sequence?`。金额与价格字段类型为 `number`（精度边界见 1.6）。

### 1.3 共享策略表单与任务监控（features/strategy）

- `StrategyForm.tsx`：props 接收目录数据与受控的 `{strategy, version, parameters}`；选择策略后按 `parameters` 动态渲染 number 输入（`min`/`max` 校验、integer 取整），未填参数不提交该字段（服务端用默认值）；展示主周期、预热根数、默认持有期。目录加载与错误态由调用方传入。
- `useRunPolling.ts`：输入 `kind`、`runId`（为 null 时不轮询）；2 秒固定间隔 `getRun`，返回最新 `RunStatus`；到达 `SUCCEEDED/PARTIAL_SUCCEEDED/FAILED/CANCELLED` 终态停止；连续 3 次网络错误停止并进入错误态；组件卸载即停。内部用世代 ref 防止旧轮询响应污染新任务状态（沿用 ChartWorkspace 模式）。
- `RunMonitor.tsx`：状态条组件——状态徽标、`data_version`、`attempts`、取消按钮（PENDING/RUNNING 时可用，调用 `cancelRun`）、错误 message。

### 1.4 扫描视图（features/scan）

`ScanPanel.tsx` 单面板三段式（TV 参考仅体现在状态条与结果表格的视觉样式）：

1. 表单：`StrategyForm` + 日期输入 `from`/`as_of`（`<input type="date">`，提交时转 `YYYY-MM-DDT00:00:00Z`，校验 from ≤ as_of 且跨度 ≤ 20 年）+ scope（SSE/SZSE/BSE 复选框，全不选=全部交易所；`active_only` 默认勾选；`limit` 默认 5000）。提交用 `crypto.randomUUID()` 生成幂等键，成功后记录 `run_id`。
2. `RunMonitor`：轮询状态，可取消。
3. `ScanResults.tsx`：终态为 `SUCCEEDED/PARTIAL_SUCCEEDED` 时，先请求 `fetchSnapshotPage({strategy, strategy_version, parameters_hash 来自 key? 无 → 用 run 状态里的 snapshot_id + strategy + version, limit: 100})`。分页规则：保存首响应的 `key`，续页请求携带 `key` 中 `snapshot_id/strategy_id/strategy_version/parameters_hash` 与 `after_sequence`；无 `next_sequence` 即结束。行内展示完整 `instrument`、`name`、`signal_time`（UTC 转本地时区），行可点击 → `onSelectInstrument(instrument)`。`failures` 数组默认折叠，`PARTIAL_SUCCEEDED` 时给出醒目计数，展开后展示 `instrument/code/message/retryable`。结果加载失败保留已加载行并提示（沿用分页失败语义）。

定位快照的实现取：优先用轮询终态响应中的 `snapshot_id` 直接作为定位参数，`strategy`/`strategy_version` 用任务输入回填；不使用"仅按 strategy 查 latest"的模糊路径，避免拿到其他任务的快照。

### 1.5 回测视图（features/backtest）

`BacktestPanel.tsx`：

1. 表单：证券选择（默认 `selectedSymbol`，点击"更换"复用 `InstrumentSearch` 弹层选择完整身份）+ `StrategyForm` + `start`/`end` 日期（同 1.4 校验）+ config 字段组：
   - `initial_cash`（元，1 千–10 亿）、`cash_fraction_bps`（1–10000，默认 10000）、`commission_bps`/`minimum_commission`/`stamp_duty_bps`/`transfer_fee_bps`/`slippage_bps`（0–10000bps，预填 3/5 元/5/0/5，标注"演示值，非费率建议"）、`lot_size`（默认当前证券 `lot_size`，可改）、`hold_bars`（可留空=策略默认，占位符显示目录 `default_hold_bars`）。
   - 提交换算：元 ×10000 → `initial_cash`/`minimum_commission` 缩放整数；bps 原值直传。
2. `RunMonitor` 轮询与取消。
3. 结果区（`SUCCEEDED` 后）：summary 指标卡（收益/年化/最大回撤/胜率/盈利因子/平均持有/平仓笔数/期末持仓；null 比例显示"—"，比例 ×100 显示 %）+ `EquityChart` + `OrdersTradesTables`。

`EquityChart.tsx`：Lightweight Charts 单 pane `LineSeries`；挂载后以 `limit:1000` 连续 `fetchRunPage('backtest', runId, 'equity', after)` 拉取直至无 `next_sequence`（最多 20 年日线约 5000 点 ≤ 6 页），期间显示加载提示；金额点 ÷10000 后入图。卸载取消进行中请求。

`OrdersTradesTables.tsx`：两个折叠表格（订单 / 成交），默认各加载 1 页（limit 100），"加载更多"用 `next_sequence` 续读；价格与金额列 ÷10000 展示，`side` 1=买 2=卖，`final_reason` 0/1/2 映射为 待执行/已成交/无下一根Bar。

### 1.6 数值与错误语义

- 缩放换算集中在 `api/client.ts` 导出的 `toScaled(yuan)` / `fromScaled(scaled)`（纯函数，单测覆盖），UI 不散落换算。
- 精度边界：所有缩放整数 ≤1e13 时 Number 精确；`initial_cash` 由表单上限 1e9 元保证；服务端返回的其他金额（权益、成交额）在个人使用规模内远低于 2^53，若出现非安全整数按展示层四舍五入并在界面标注（防御性，不阻塞）。
- 错误：复用 `request<T>` 的 Envelope 映射；`IDEMPOTENCY_CONFLICT` 消息为"任务输入与已有任务冲突，请重新提交"，用户再次点击创建时自动换新 UUID；其余 message 原样展示于所在面板错误态。

## 2. 文档同步

- `docs/design/web.md`：`owns` 增加 `web/src/features/strategy/`、`web/src/features/scan/`、`web/src/features/backtest/`；职责、状态不变量、测试章节补充扫描/回测视图与轮询语义。
- `docs/design/README.md`：源码地图与模块职责行补三个新目录。
- HTTP 契约无变化，不改。

## 3. 测试策略

测试先行，Vitest + Testing Library，mock `api/client`：

| 文件 | 覆盖 |
|---|---|
| `client.test.ts` 扩展 | 新端点 URL/method/body 构造、`toScaled`/`fromScaled` 换算与边界、分页参数 |
| `StrategyForm.test.tsx` | 目录渲染、参数 min/max/integer 校验拦截、留空参数不上送 |
| `useRunPolling` 相关（经面板测试） | fake timers 轮询节奏、终态停止、卸载停止、3 次网络错误停止 |
| `ScanPanel.test.tsx` | 提交体（UUID mock、日期转 UTC、scope 默认）、快照 key 固定续页、failures 折叠、行点击回调、409 提示 |
| `BacktestPanel.test.tsx` | config 换算与默认值、lot_size/hold_bars 默认、summary 空值渲染、equity 连续拉取、订单"加载更多" |
| `App.test.tsx` / `chartData.test.ts` 扩展 | `tab` 白名单、切换视图保持 symbol |

## 4. 前端门禁

`npm --prefix web run check`（type check + test + coverage，沿用现有脚本与阈值）；无 Go 改动，`go test ./...` 与 `bash scripts/verify.sh` 照常跑全局门禁。

## 5. 实施顺序

1. commit 1：`client.ts` 扩展 + 换算纯函数 + `chartData` tab 白名单 + `App` 视图路由（含测试）
2. commit 2：`features/strategy`（StrategyForm/useRunPolling/RunMonitor）+ `features/scan` ScanPanel/ScanResults（含测试）
3. commit 3：`features/backtest` BacktestPanel/EquityChart/OrdersTradesTables（含测试）
4. commit 4：文档同步（web.md、README.md）+ 变更记录状态更新

每步可编译、测试全绿；全部验收通过并经独立评审后才置 implemented。

## 6. 回滚

纯前端新增，无持久化与接口变化；回滚即恢复前端文件，localStorage 键残留无副作用。
