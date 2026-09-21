---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["internal/port/"]
related: []
---

# 应用端口设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `internal/port` |
| 最后更新 | 2026-09-20 |

## 1. 职责与非职责

本模块定义应用层与行情来源、持久化、任务队列、结果读取、证券目录、遥测和事件发布之间的稳定边界，同时提供经过校验的 DTO、状态枚举、分页、失败和身份规则。

端口不包含 GORM、SQL、HTTP DTO 或具体 Broker 类型，也不为只有一个简单实现的内部函数预先抽象接口。

## 2. 能力族与使用者

| 能力族 | 主要接口 | 语义 |
|---|---|---|
| 行情读取 | `MarketData`、`MarketChangeReader` | 完成版本、单证券/批量 Dataset、证券范围和版本变化 |
| 行情写入 | `MarketDataWriter` | 校验一个不可变发布候选并返回完成版本 |
| 外部来源 | `DailyMarketSource` | 生产刷新获取日线与复权因子 |
| 任务队列 | `JobQueue`、`IdempotentRunReader` | 入队、领取、续租、重试、取消和过期回收 |
| 结果存储 | `RunStore` | Run 状态、回测结果页和原子完成/失败 |
| 快照 | `SignalSnapshotStore` | 按完整 SnapshotKey 选择或继续不可变快照 |
| 证券目录 | `InstrumentCatalog` | 活跃证券搜索和完整身份查询 |
| 自选清单 | `WatchlistStore`、`DailyQuoteReader` | 单用户自选增删查（插入序、上限 100）与按证券行 ID 批量读取最新日线报价 |
| 行情看板 | `ChartBoardStore` | 单用户看板增删改查与激活（id 升序、上限 20、恰一激活） |
| 可观测性 | `Telemetry` | 白名单阶段耗时与重试计数 |
| 事件 | `EventPublisher` | 按稳定事件身份持久化完成事件 |

应用层消费接口，MySQL 与 Broker 提供实现；领域模块只消费端口 DTO 中的领域对象，不依赖接口本身。

## 3. 依赖和边界

端口只依赖 Go 标准库和 `internal/market`、`internal/backtest` 等必要领域结果。接口参数始终带 `context.Context`；实现必须遵守取消契约并返回独立可修改的响应副本。

可选扩展接口表达实现能力，例如幂等前读和行情变化分类。生产装配不能用功能不完整的包装器隐藏已需要的扩展能力。

`DailyMarketSource` 是当前唯一的外部行情来源边界，只返回日线和复权因子；周线由应用层生成，旧库迁移在进程内直接转换旧表行、不经过外部来源。

## 4. 核心模型与不变量

### 4.1 Run

- Run 保存复现异步执行所需的策略、引擎、数据版本、规范化请求和输入摘要。
- 状态固定为 PENDING、RUNNING、SUCCEEDED、PARTIAL_SUCCEEDED、FAILED、CANCELLED。
- 新入队 Attempts 为 0；持久化队列赋值领取次数，范围 0–4。
- RUNNING 必须同时持有非空 owner/token；非运行状态不得残留租约。

### 4.2 行情批量读取

- `InstrumentScope` 显式限制为最多 5000 个证券；空交易所集合表示所有支持交易所。
- `BatchRequest` 必须包含主周期、UTC 闭区间、正版本和有界 lookback；辅助周期合法、去重且不能等于主周期。
- `Bundle` 返回同证券、同版本的主/辅助 Dataset、因子、公司行动和质量；实现返回独立副本。

### 4.3 发布与摘要

- `MarketWriteBatch` 至少含一类变化，所有 Bar/Action 必须属于批次证券。
- Digest 由规范化批次计算，调用者输入切片仍归调用者所有，Writer 留存前必须复制。
- 数据版本不参与内容摘要，由存储在原子发布时分配。

### 4.4 快照与分页

- 首页面可不带 SnapshotID 选择最新匹配快照；续页必须带返回的精确 ID。
- SnapshotID 不能绕过 StrategyID、Version、ParametersHash 和非零 AsOf 的匹配。
- 同证券不能同时出现在成功 rows 与 failures，快照证券行不得重复。
- PageRequest 使用非负排他序号，limit 为 1–1000。

### 4.5 自选清单

- `WatchlistEntry` 携带完整证券身份与 `t_instruments` 行 ID，行 ID 仅供报价批量读取，不外泄到 API。
- `DailyQuote` 的 Close/Change/ChangePercent 为指针语义：null 表示对应 bar 不可用，零值不承载"无数据"含义。
- `WatchlistStore.Add` 幂等，`Remove` 对不存在条目也成功；上限由 `MaxWatchlistItems`（100）约束，应用层校验。

### 4.6 行情看板

- `ChartBoard` 携带服务端生成的数值 id、name 与 `json.RawMessage` config；`ChartBoardState` 为全量状态信封（boards 按 id 升序 + active_id）。
- `ChartBoardStore` 的创建即激活、更新不触碰激活状态、删除激活看板时由存储事务激活剩余 id 最小者。
- 上限由 `MaxChartBoards`（20）约束，应用层校验；`ErrChartBoardNotFound` 区分未知 id。

### 4.7 身份与时间

- 所有不透明身份按 UTF-8 字节验证，不 trim，不折叠大小写或尾空格。
- UTC 时间必须明确 Location 且最多微秒精度；允许零时间的字段由各 DTO 单独声明。
- port 错误保留稳定分类，适配器错误使用 `%w` 包装以便 API 和 Worker 判断。

## 5. 主要流程

```text
应用构造并 Validate DTO
  → 调用端口接口
  → 适配器再次保护存储/来源边界
  → 返回领域值或稳定错误
```

端口校验是跨适配器契约，不替代数据库唯一约束、事务条件或领域构造校验。

## 6. 失败、取消和一致性语义

- 接口返回错误必须携带操作上下文并保留可分类原因。
- `record not found` 仅在契约明确允许时转为空结果，否则返回 NotFound 类错误。
- `BatchDatasets` 区分全局数据库错误和单证券数据错误；不能因一只证券失败丢弃其他结果。
- 租约写操作必须绑定 run/token，失租与普通存储故障是不同错误。

## 7. 性能与安全约束

- 所有列表、范围和分页 DTO 有明确上限及稳定排序语义。
- RequestJSON、租约凭据、幂等键和原始失败不能直接暴露给 API。
- 遥测只接受声明字段，禁止承载完整配置、请求或原始错误正文。

## 8. 测试与验收证据

契约测试覆盖身份字节边界、UTC 微秒、枚举全集、批量范围、快照身份、分页、摘要稳定性和响应所有权。

```bash
go test ./internal/port -cover
```

## 9. 相关文档

- [系统设计](../../architecture/system-design.md)
- [应用层设计](application.md)
- [MySQL 设计](infrastructure/mysql.md)
- [Broker 设计](../pkg/broker.md)

## 更新进度端口

`RefreshProgressWriter.SaveRefresh` 接收独立观察快照，不决定执行范围。`RefreshProgressReader` 提供 LatestRefreshRun、ListRefreshRuns、GetRefreshRun、ListRefreshFailures；最新按实际开始时间，历史按持久化 id 分页。`RefreshRun` 保存批量任务状态/数量/时间，`RefreshFailure` 只暴露证券身份和稳定错误码，`RefreshReceipt` 返回 ACCEPTED、run_id 与 progress_available。证券失败明细与失败数量必须一致、身份唯一且符合范围上限；准备中 total 可为空，状态与结束时间匹配。此端口与计算 Run/租约系统相互独立。

## 批量刷新回执

`BatchRefreshReceipt` 包含 stock/futures 两个 `RefreshReceipt`，每类 status 为 ACCEPTED、ALREADY_RUNNING、DISABLED（仅期货）或 FAILED。仅 ACCEPTED 有 run_id，progress_available 描述观察存储是否可用；FAILED 只有固定 REFRESH_UNAVAILABLE 错误码。此 DTO 不创建父任务或改变进度存储关系。
