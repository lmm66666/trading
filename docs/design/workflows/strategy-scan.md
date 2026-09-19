---
kind: explanation
status: baseline-review
authority: code-derived
baseline_revision: 0aadb823d45701cd3c1d702f510bf97420bb4fc1
owns: []
related: []
---

# 策略扫描：从一组证券得到可复查的候选名单

你指定策略、证券范围和时间窗口，系统使用已入库行情逐只计算，返回在窗口内最后一根可用主周期 Bar 上发出买入信号的证券。日线策略的 Bar 是日线，周线策略则是周线。扫描不下单，也不计算收益。

**先看结果含义，再深入实现：** [一个完整例子](#完整走一遍) · [窗口与预热](#时间窗口与预热) · [数据流](#数据怎样流动) · [接口与表](#接口和表之间怎样关联) · [失败与复查](#失败与复查) · [证据与待确认事项](#证据与待确认事项)。

本文解释当前代码；它拥有扫描流程的完整阅读说明，不定义某个策略的阈值。日线 B1 的具体条件见 [日线 B1](../strategies/daily-b1.md)，既有技术契约见 [应用层设计](../internal/application.md)。

## 完整走一遍

假设你以日线 B1 扫描三个证券，截止 D4。以下动作是假设策略已经给出的决策，用来解释扫描如何收集结果，不用来证明 B1 本身的判断。

| 证券 | 读到的数据与决策 | 结果 |
|---|---|---|
| 甲 | D3 买入，D4 不买入 | 不入选；扫描不是收集区间内全部历史信号 |
| 乙 | 最后可用日线是 D4，决策为买入 | 入选，`signal_time` 为 D4 的收盘时刻 |
| 丙 | 最后可用日线是 D3，决策为买入，D3 在请求范围内 | 当前实现允许入选，`signal_time` 为 D3；截止时间不保证数据新鲜度 |

创建任务时，系统固定证券集合、完整行情版本、策略版本、参数和引擎版本。假设固定行情版本为 42，排队时又发布了 43，本任务仍使用 42。计算结束后保存一份不可变快照；分页读这份快照，不随着新任务完成而换页到另一份名单。

这使结果可以解释和复查：既要知道“用了哪个策略”，也要知道“用了哪一版数据、哪些参数和哪些证券”。当前代码是否满足已确认的业务期望，还需要下面的证据与待决事项共同判断。

## 时间窗口与预热

`from` 和 `as_of` 定义请求窗口。每只证券最终使用的主周期 Bar 收盘时间必须落在窗口内；窗口内没有可用的最后一根时，该证券失败。窗口中间曾经发信号但最后一根不买入，不进入名单。

为形成指标，批量加载还请求策略声明的预热根数。扫描实际调用 `ReplayLatest` 回放整个加载序列，包括预热部分，因此预热阶段也可能建立有状态策略的候选。

**这里存在一项待裁决的文字／实现差异：** [应用层设计](../internal/application.md) 写的是“暖机行情只用于指标预计算”，而当前扫描实现会把它送入策略回放。日线 B1 的上涨候选因此可能开始于 `from` 之前。本文记录实际实现，不修改原契约，也不据此认定哪一方错误；如果要把 `from` 定义为策略状态的重置点，需要单独确定业务语义并验证。

`as_of` 是数据读取上界；当前没有“所有证券最后一根必须等于截止日”的强制检查。使用结果时看 `signal_time`。如何处理停牌、节假日和来源更新延迟，不能只由日期是否相同推断。

## 数据怎样流动

```text
外部来源 → 行情采集与完整版本发布 → MySQL 的行情和因子
  → 创建扫描，固定输入 → 持久化任务排队
  → Worker 领取任务 → 批量读取固定版本的数据
  → 每只证券独立实例：计算指标、按时间回放、取最后决策
  → 快照、结果行、任务终态和完成事件一起提交
  → 使用固定 SnapshotID 分页读取
```

扫描读取已经入库的数据，不逐证券临时请求外部行情。外部来源、日周线生成和行情发布规则归 [行情采集与版本](market-data.md) 与 [行情领域设计](../internal/market.md)。

应用层先核验保存的输入摘要与数据版本，批量读取主周期及辅助周期，再为每个证券创建独立策略实例。这样一只证券的回撤状态不会串到另一只证券。结果按完整证券身份稳定排序；它不是按信号强弱或预期收益排名。

当前每次最多 5000 个证券，扫描计算 worker 为 1–64，时间范围最多 20 年。这些是实现上限，不是测得的个人使用量或运行频率；本次不据此增加缓存或新的服务。

### 何时能复用上次的计算

同一策略配置、时间起点、证券集合、截止时间及可用前快照下，当前服务可以复用未变化证券的结果。前快照中的失败证券重新尝试；仅 Bar 修订时重新计算受影响证券；因子或公司行动变化则全量重建。来源无法给出变化分类，或没有可复用前快照时，也全量计算。

这不会改写旧快照，而是生成本次任务的新快照。复用范围由 [scan_worker.go](../../../internal/application/scan_worker.go) 的 `mergeBase` 与 [scan_service.go](../../../internal/application/scan_service.go) 的配置摘要共同确定。

## 接口和表之间怎样关联

### 调用顺序

| 你要做的事 | 接口与要点 |
|---|---|
| 创建扫描 | `POST /api/v1/scan-runs`，传 `strategy=daily_b1_buy`、`strategy_version=1`、`from`、`as_of`、`scope` 和 `idempotency_key`；JSON 中版本为字符串 |
| 查询执行状态 | `GET /api/v1/scan-runs/:run_id`，成功或部分成功后取得 `snapshot_id` |
| 读取候选名单 | `GET /api/v1/signal-snapshots/latest`；指定该任务的 `snapshot_id` 定位结果，保留返回的 key |
| 继续读取下一页 | 携带相同快照及其策略、版本、参数 hash，以及服务端返回的 `next_sequence` 作为 `after_sequence` |
| 取消任务 | `POST /api/v1/scan-runs/:run_id/cancel` |

仅按策略查询 latest 可能得到其他参数或其他任务的最新结果，不能用它替代指定任务的结果定位。相同幂等键重试相同请求复用原任务；改变请求内容但继续使用同一键会冲突。完整请求样例和响应字段见 [HTTP 契约](../../standards/http-api.md)。

### 持久化关系

下表是代码模型中的逻辑关联，不表示数据库已经声明了外键。

| 表 | 保存什么 | 怎样关联、为什么这样存 |
|---|---|---|
| `t_instruments` | 证券身份 | 内部证券 ID 被行情和结果行引用，避免只用六位代码造成交易所歧义 |
| `t_market_bars` | 各证券日线及修订 | 按 `instrument_id`、周期、收盘时间定位，按版本有效区间读取任务锁定的历史视图 |
| `t_adjustment_factors` | 复权因子及有效范围 | 以 `instrument_id` 关联证券，并结合生效时间和数据版本选取因子 |
| `t_compute_runs` | 扫描请求、固定输入、执行状态与租约 | `run_id` 唯一；`(kind, idempotency_key)` 唯一约束避免同一请求重复创建任务 |
| `t_signal_snapshots` | 快照头、数据版本和失败分类 | `snapshot_id` 与 `run_id` 各自唯一；当前扫描使用任务 ID 作为快照 ID |
| `t_signal_snapshot_rows` | 入选证券、信号时间和原因 | `snapshot_id` 关联快照头，`instrument_id` 关联证券；未入选且未失败的证券不生成结果行 |

结果行的 `(snapshot_id, instrument_id)` 唯一约束避免一份快照重复收录同一证券；`(snapshot_id, sequence)` 唯一约束为稳定分页提供顺序。行情的 `(instrument_id, timeframe, close_time, revision)` 唯一约束保留不同修订，不能把同一日期的不同版本随意覆盖成一条。

快照头的 `idx_snapshot_latest` 依次包含策略、版本、参数 hash、状态和截止时间，服务相应的快照检索；具体查询计划与性能仍需数据库验证。结果发布由存储层把快照、行、任务终态和完成事件放在同一事务中完成，避免任务显示成功但结果尚未完整写入。完整索引和事务规则归 [MySQL 设计](../internal/infrastructure/mysql.md)。

## 失败与复查

| 你看到什么 | 应怎样理解 |
|---|---|
| 创建请求被拒绝 | 检查策略版本、参数、日期与证券范围；不会静默换成另一组输入 |
| 某证券不在结果，也不在失败列表 | 本次最后决策没有买入信号；不代表整个窗口内从未发过信号 |
| 某证券出现在失败列表 | 缺失数据或计算失败，不能归为“策略不满足” |
| `PARTIAL_SUCCEEDED` | 快照同时保存可发布结果与失败分类，必须结合 failures 使用 |
| 客户端请求结束或断开 | 不等同于显式取消已持久化的任务 |
| 显式取消、租约失效或执行失败 | 不允许按成功路径发布结果；重试与任务恢复归应用层和持久化契约 |

复查某个结果时，先取得任务及快照身份，再核对策略版本、参数、证券集合、行情版本、引擎版本和时间窗口。仅重跑“最新数据”不能证明旧结果错误，因为输入可能已改变。

## 实现入口

| 想检查什么 | 入口 | 详细技术规则 |
|---|---|---|
| 请求如何固定、重试是否重复创建 | [scan_service.go](../../../internal/application/scan_service.go)：`Create` | [应用层设计](../internal/application.md) |
| 只取最后决策、单证券失败、增量复用 | [scan_worker.go](../../../internal/application/scan_worker.go)：`replay`、`scanBatch`、`mergeBase` | [策略模块设计](../internal/strategy.md) |
| 请求和结果字段 | [create_scan_run.go](../../../api/create_scan_run.go) 及 [HTTP 契约](../../standards/http-api.md) | HTTP 契约拥有字段全集及精确校验规则 |
| 结果原子发布 | [run_store.go](../../../internal/infrastructure/mysql/run_store.go)：`CompleteScan` | [MySQL 设计](../internal/infrastructure/mysql.md) |
| 快照及结果行索引 | [快照模型](../../../internal/infrastructure/mysql/signal_snapshot_model.go)、[结果行模型](../../../internal/infrastructure/mysql/signal_snapshot_row_model.go) | 同上；本文的表关系是解释，不代替完整持久化契约 |

## 证据与待确认事项

| 要验证什么 | 既有测试入口 | 能证明到哪里 |
|---|---|---|
| 批量读取、有界计算 | [scan_service_test.go](../../../internal/application/scan_service_test.go)：`TestScanLoadsEachTimeframeInBoundedBatches` | 应用层调用方式，不代表生产数据库耗时 |
| 成功信号与单证券失败一起保留 | 同文件：`TestScanPublishesFailuresWithoutDroppingSuccessfulSignals` | 服务层行为，不替代真实 MySQL 集成检查 |
| 快照分页固定，因子变化重算 | 同文件：`TestScanIncrementalPinsPagesAndInvalidatesFactors` | 已有场景中的复用与失效规则 |
| 同一幂等键保留原证券集合、拒绝输入变化 | 同文件：`TestScanIdempotencyKeepsOriginalUniverseAndRejectsInputConflict` | 请求重试行为 |
| 信号不超出固定窗口 | 同文件：`TestScanNeverPublishesSignalsOutsidePinnedWindow` | 窗口边界，不证明截止时数据新鲜 |
| 甲乙丙示例、陈旧最后一根仍可入选、预热参与策略回放 | `replay` 与 [ReplayLatest](../../../internal/strategy/replay.go) 的静态核对 | 这些说明性场景没有在本次新增为自动化测试 |

需要你决定的两件事：**是否要求最后一根数据足够新鲜；预热能否建立策略状态。** 前者当前没有业务新鲜度阈值；后者存在上述契约措辞与实现差异。选择以后才进入相应规则和代码变更，本批保持原行为。

本文依据本地 `0aadb82` 代码整理，尚未把这些未决行为确认为新契约。实际检查结果见 [本批验证记录](../../changes/archive/2026-09-19-readable-design-pilot/verification.md)。
