# 应用层

应用服务只依赖领域值对象与 port，不导入 GORM、Gin 或旧 K 线模型。DTO 中的时间使用 UTC，传输层 JSON 按 RFC3339 输出。

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
