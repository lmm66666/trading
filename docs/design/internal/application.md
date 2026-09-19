---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["internal/application/"]
related: []
---

# 应用层设计

想先理解扫描结果、时间窗口和数据流，请读 [策略扫描流程](../workflows/strategy-scan.md)。本文保留应用层的依赖、执行和生命周期契约；扫描说明中已列出预热表述与实现的待裁决差异。

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `internal/application` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

应用层编排行情采集与查询、图表、证券搜索、扫描、回测、持久化 Worker 和调度生命周期。它负责固定可复现输入、协调端口、传播取消、分类重试，并把领域结果交给原子发布边界。

应用层不实现指标、策略、撮合、外部协议解析或数据库事务；这些职责分别属于领域模块和适配器。

## 2. 对外能力与使用者

- API 使用行情查询、图表、证券搜索、回测和扫描服务。
- 组合根使用行情采集、股票/期货调度器和持久化 WorkerPool。
- 服务输入输出只使用领域值对象、应用 DTO 和 `internal/port` 契约，不暴露 ORM Model。

## 3. 依赖和边界

应用层只依赖 `market`、`indicator`、`strategy`、`backtest` 与 `port`，不导入 Gin、GORM、具体 Broker 或旧 K 线模型。业务算法留在领域模块，外部协议解析留在适配器，事务与并发写入仲裁留在仓储。

可选扩展端口通过能力检测使用；缺少增量变化分类时安全全量计算，不能用不完整包装器隐藏生产仓储已经具备的幂等读取或变化分类能力。

## 4. 核心模型与不变量

- 回测和扫描创建时固定策略定义、参数、窗口、证券集合、行情版本和引擎版本，并以 canonical SHA-256 保护完整输入；执行前重新核验保存输入和摘要。
- 扫描为每个证券创建独立策略实例，结果按证券稳定排序；后续分页始终绑定首次返回的 SnapshotID。
- 暖机行情只用于指标预计算，账户和信号窗口从请求起点开始，禁止读取未来数据。
- 行情刷新只消费 `port.DailyMarketSource` 的日线与复权因子；周线由应用层对最终日线调用 `market.AggregateWeekly` 确定性生成。
- 当前生产刷新不获取公司行动。写入是增量 upsert，缺省 Bar、因子或公司行动不表示删除；端口尚无权威快照撤回语义。
- DTO 时间使用 UTC；跨持久化身份按微秒规范化，与 MySQL `DATETIME(6)` 一致。

## 5. 主要流程

### 5.1 回测、扫描与 Worker

创建回测时锁定一次最新 COMPLETE 行情版本；执行阶段先校验数据质量、归属和版本，再计算指标与运行引擎。取消、引擎错误或失效租约均不能发布结果。

扫描最多固定 5000 个证券，通过一次批量读取加载主/辅助周期。只有同一 AsOf、配置、证券集合和前快照身份可以增量复用；纯 Bar 修订只重算 dirty 证券，因子修订或公司行动修订使整个快照失效。缺少变化分类能力时全量重建。

幂等提交先读取已存在 Run，再由数据库唯一约束仲裁并发首次提交。碰撞后重读胜出者，核验其完整保存输入和摘要；相同键但用户输入不同返回 `port.ErrIdempotencyConflict`，其他存储错误不得伪装成碰撞。

WorkerPool 使用固定 worker、周期 reaper 和活跃任务续租。续租周期为租期三分之一；持久 Attempts 决定 250ms、1s、4s 退避，第四次失败进入终态。应用关闭先取消根 context、等待 WorkerPool 和调度器退出，再关闭数据库。

### 5.2 行情刷新

首次刷新读取 HistoryStart 至当前 UTC 的日线；普通股票刷新从最近 20 根已存日线起点重抓，期货可由配置选择全历史刷新。来源日线与已存数据合并后生成周线，再获取并规范化复权因子，校验范围、重复时间、因子覆盖和最终日周一致性，最后计算 canonical 批次并一次发布。

当前生产装配把同一 `rate.Limiter` 注入股票和期货来源适配器，在 HTTP Transport 边界共享限频；`MarketIngestionConfig.Limiter` 是额外的可选并发协调钩子，生产配置目前不注入。两者不能在设计中混写成已经启用的同一机制。

同一证券的并发刷新在单进程内返回 `ErrRefreshAlreadyRunning`。多进程部署仍必须保持单个行情发布者；当前没有跨进程刷新租约或基线版本 CAS。

### 5.3 行情查询与调度

行情查询的零版本只解析一次最新 COMPLETE 版本，之后始终读取该确切版本。零 To 默认当前 UTC，零 From 默认向前 20 年，零 Limit 默认 5000；返回最近 Limit 根并按收盘时间升序。复权查询缺因子或版本不符直接失败。

调度器使用 1–64 个固定 worker，范围最多 5000 个去重证券。`Start` 立即执行一次后按 interval 同步循环；逐证券失败进入汇总但不终止后续周期，全局读取或生命周期错误才结束循环。

## 6. 失败、取消和一致性语义

- context 取消贯穿端口、领域 Replay、Worker 和调度器；取消或失租不重试、不发布。
- 只有明确 temporary、timeout 或 transient connection 错误可重试。落库 `ErrLeaseLost` 只结束当前任务，其他落库错误传播并终止受影响生命周期。
- 行情空响应、已知日期丢失、重复时间、非法因子或最终日周不一致均返回 `ErrIncompleteMarketData`，任何一步失败都不发布部分批次。
- 全局数据版本由 writer 原子分配；应用层预检查不能替代数据库唯一约束和条件更新。

## 7. 性能与安全约束

- 时间范围最多 20 年，单次 Bar/因子上限使用 port 的 10000 约束；扫描证券最多 5000，所有 worker 数有界。
- 批量场景调用批量 port，不按证券产生无界数据库往返或 goroutine。
- `SlogTelemetry` 只接收声明的阶段、身份、版本、耗时和计数字段，不记录请求体、配置或原始错误文本；当前 `queue_wait` 只表示 Claim 调用等待。

## 8. 测试与验收证据

```bash
go test -race ./internal/application -run 'Test(Refresh|Prices|MarketScheduler)'
go test ./internal/application ./internal/port -cover
go test ./...
go vet ./...
```

测试覆盖输入摘要、固定版本、增量扫描、租约与取消、批量上限、日线合并、本地周线生成、复权因子覆盖、可选 Limiter 和调度生命周期。

## 9. 相关文档

- [系统设计](../../architecture/system-design.md)
- [领域地图](../README.md)
- [应用端口设计](port.md)
- [行情领域设计](market.md)
- [MySQL 适配器设计](infrastructure/mysql.md)
- [Broker 设计](../pkg/broker.md)
