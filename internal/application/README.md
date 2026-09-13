# 应用层

应用服务只依赖领域值对象与 port，不导入 GORM、Gin 或旧 K 线模型。DTO 中的时间使用 UTC，传输层 JSON 按 RFC3339 输出。

## 回测、扫描与持久化 worker

`NewBacktestService` 注入 Registry、BacktestEngine、MarketData、JobQueue、RunStore 和 ComputeConfig；EngineVersion 必须由装配层显式提供。Create 验证稳定策略定义、补齐默认参数，锁定一次最新 COMPLETE 版本，并对策略定义、完整成本/执行配置、UTC 日期、参数和数据/引擎版本计算 canonical SHA-256。JSON 对象键排序，浮点使用固定十进制表达，整数金额不转浮点。执行重新校验保存的摘要，只读取锁定版本；数据质量/归属/版本检查在指标和引擎前进行，暖机行情只用于预计算指标，账户从请求窗口开始。取消、引擎错误和失效租约均不能发布结果。

`NewScanService` 通过 ScanConfig 注入1–64个固定 worker。Create 固定证券集合（最多5000）、窗口、参数和数据版本；日期规范化为 UTC 微秒，与 MySQL DATETIME(6) 身份一致。Execute 通过一次 BatchDatasets 同时加载主/辅助周期，为每只证券创建新的策略实例，并记录成功信号与脱敏失败。无买入信号为 skipped；输出按 InstrumentID 排序后由 RunStore 原子发布。每个证券和仓储批次边界检查执行 context，Replay 在 Bar 边界再次检查。

扫描的 ParametersHash 覆盖策略定义、参数、引擎、窗口起点、scope 和固定证券集合。只有同一 AsOf/配置/证券集合的前快照可增量复用；更换窗口或配置完整重算。MarketData 若同时实现 `port.MarketChangeReader`（MySQL 实现已提供），纯 Bar 修订只重算 dirty 证券，并重试前快照失败项；任一因子或公司行动修订使整个快照失效。缺少变化分类能力时安全全量重建。前快照 ID 被纳入任务摘要，读取每个后续页均绑定该 ID，不能随着新发布切换快照。Latest 的续页同样要求 SnapshotID。

JobQueue 若实现 `port.IdempotentRunReader`（MySQL 实现已提供），重复提交会在解析新行情版本或新证券集合前读取原 Run；相同幂等键的配置冲突返回 `port.ErrIdempotencyConflict`（兼容 ErrInvalidPortValue）。Enqueue 通过数据库唯一约束仲裁并发首次提交；碰撞后应用重读胜出 Run，严格解码全部保存字段并核验其摘要，再比较原用户请求的完整参数、范围、scope和执行配置。系统在竞争期间解析到的新数据/引擎版本、证券集合或前快照不会误判同一用户请求为冲突，复用始终采用胜出者已锁定的输入；其他存储错误不按碰撞处理。生产装配应保留两个扩展端口能力，不能用功能不完整的包装器隐藏它们。

Enqueue 成功返回同样经过完整保存字段、摘要与用户请求核验，包括本次新建候选和同 hash 并发返回的已有 Run；不能只在前查命中或显式碰撞错误时验证，否则旧格式或异常保存请求可能绕过校验。

`NewWorkerPool` 接收按 RunKind 分派的 Execute 方法。Run 管理固定任务 worker、周期 reaper 和每个活跃任务的续租 goroutine，退出前全部等待结束。续租周期为租期的三分之一，并读取取消状态；失租或续租状态不确定立即取消计算，不尝试用旧 token 写入结果。应用关闭时取消根 context，再等待 pool 返回，最后关闭数据库。每次领取的持久 Attempts 决定250ms、1s、4s退避；只有明确 temporary、timeout 或 transient connection 错误重试，第四次失败进入终态。任务取消/失租不重试；关闭中断保留租约供后续接管。

当 handler 返回后、续租观察停止到 Retry/Fail 落库之间发生取消或重领，存储返回 ErrLeaseLost 只结束当前任务，worker pool 继续处理无关任务。其他落库错误仍传播并触发生命周期退出，不以失租为由吞掉存储故障。

`SlogTelemetry` 仅接受声明的阶段、身份、版本、耗时和计数字段；不接收请求体、配置或原始错误文本。覆盖行情加载、校验、指标、策略、引擎和持久化。当前 queue_wait 测量 Claim 调用等待，Run 端口尚无入队时间戳，不能将它解读为完整队列驻留时间。

## 行情采集

`NewMarketIngestionService` 接收 MarketSource、MarketData、MarketDataWriter，以及显式 Source、HistoryStart、Clock 和可共享的 UpstreamLimiter。生产装配应注入已有 `pkg/indicator.Limiter`；同一 limiter 可服务多个来源调用。时间范围最多20年，每个周期最多10000根 Bar/因子，公司行动最多10000条。

`Refresh` 首次请求 HistoryStart 至当前 UTC 时点的日线与周线。已有行情按最近20根实际日线的起点重抓，并覆盖相应周线边界及已知跨周期缺口。没有交易日历或来源覆盖元数据时，不把自然日间隔认定为缺数，也无法证明两个周期同时缺失的内部历史日期；需要维护时扩展历史回填能力。

日/周 raw 与 QFQ 配对由 MarketSource 负责；服务再次检查领域合法性、请求范围、已知日期保留、因子覆盖和跨周期一致性。公司行动获取成功且全部校验完成后，使用 port.CanonicalMarketBatch 生成 SHA-256 批次并调用一次 Publish。空行情、部分响应、重复时间、冲突因子或任一步失败均不发布；合法空公司行动允许通过。

发布前按 writer 的 upsert 规则合并当前配置窗口内的旧 Bar 与本次 Bar，验证最终日周状态：每根周 Bar 必须有同收盘时点、同 close 的日 Bar；每个已存日线周组的最终末日必须有周 Bar。由此确保扩大请求后已知缺口确实补齐，且窗口外保留旧日线不会与新周线修订矛盾。即使批次内部一致，最终状态不一致仍零发布。本次新增且尚无已观测周收盘的未知状态，最多只适用于最终日线最新的 ISO 年/周组；任何更早的新周组都已被后续日线周组证明结束，必须有周 Bar。这个判断依据已观测数据，不依赖自然日或 Clock 推断，也不豁免此前已经识别的缺口。

如果近期重抓发现复权基准变化，则在同一次 Refresh 中重新获取整个配置历史窗口。由于 writer 使用增量 upsert，服务还覆盖窗口内已存但来源不再包含的因子断点，避免过期断点继续生效。缺省 Bar 或公司行动不代表删除；当前端口没有事件撤回或权威快照的删除能力。

同一证券的并发 Refresh 在本进程返回 ErrRefreshAlreadyRunning；全局版本由 writer 事务分配。多进程部署仍须限制行情发布者为单实例，当前端口没有跨进程采集租约或期望基线版本比较交换。

## 行情查询

`Prices` 接收证券、日/周周期、Raw/ForwardAdjusted、闭区间 From/To、Version、Limit。零版本只解析一次 LatestCompleteVersion，后续始终读取该确切版本；空库、未知证券/版本以 port.ErrMarketDataNotFound 标识。底层错误由传输层统一脱敏映射。

零 To 默认当前 UTC 时刻；零 From 默认 To 之前20年；零 Limit 默认5000。返回窗口内最近 Limit 根并按收盘时间升序，超限或非法输入失败。复权查询缺因子或因子版本不符直接失败。成交量和成交额保持原始口径；DTO 不暴露 ORM 模型。

## 调度与生命周期

`RunOnce` 使用全进程触发 guard 和1–64个固定 worker，不按证券数创建 goroutine；范围最多5000个证券，重复证券去重。逐证券成功/失败分开归集，取消时归集已启动任务并标记未启动证券，等待 worker 全部退出。

`Start` 是同步生命周期：立即执行一次，再按 interval 触发，直至取消或证券列表读取等全局错误；逐证券失败不终止后续周期。由装配层用受管理 goroutine 启动，并在关闭数据库前等待其返回。重复 Start 被拒绝；`LastSummary` 返回独立副本，便于监督循环读取结果。来源和仓储必须遵守 context 取消契约。

验证：

```bash
go test -race ./internal/application -run 'Test(Refresh|Prices|MarketScheduler)'
go test ./internal/application ./internal/port -cover
go test ./...
go vet ./...
```
