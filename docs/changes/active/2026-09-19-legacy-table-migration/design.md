---
id: CHG-2026-09-19-legacy-table-migration-DESIGN
authority: normative
approval_status: approved
---

# 目标设计：迁移数据源收敛为旧表直迁

## 1. 合并后状态

### 1.1 迁移器

`NewLegacyMigrator(db *gorm.DB)` 单参数构造；`backfillLegacy(item legacyInstrument) (port.MarketWriteBatch, error)` 不再接收 source 与 ctx：

- 对 `market.Day`/`market.Week` 各自的旧表行执行转换：`model.StockKline` 的日期解析为 UTC close time，价格列按 DECIMAL 值缩放 10000 转 `market.Price`，成交量按旧表列单位转 int64（实施时以 `model.StockKline` 列类型核对），`Trading=Tradable`。
- 每个 timeframe 的转换结果经 `market.NewDataset` 校验（升序、无重复、OHLC 与成交量合法），失败映射既有失败分类。
- `batch.Factors` 与 `batch.Actions` 为空列表；`canonicalBatch` 规范化保留（排序、去重、digest）。
- 原 L375-390 因子覆盖校验删除；bars 至少一个 timeframe 非空的约束保留。
- 锁、源快照、检查点（backfill state）、报告统计、`COMPLETE` 发布与幂等重跑语义全部不变；`Quality` 判定仍为无 `RejectedCodes`/`Failures` 即 `COMPLETE`。

### 1.2 QFQ 语义

迁移版本因子为空：`GET /api/v1/market/bars?view=qfq` 与 QFQ 图表查询对迁移版本按既有读路径行为拒绝（因子缺失错误），RAW 正常。激活 SQL 执行后触发一次全市场刷新（`POST /api/v1/market/refresh` 或调度器），刷新版本携带新浪 `qfq.js` 全量断点因子且首断点回拨到全历史首根 bar，此后最新版本 QFQ 全历史可用。该修复链路为生产既有行为，本变更不修改。

### 1.3 包结构

- `pkg/broker`：删除 `eastmoney_market.go`、`eastmoney_market_test.go` 与 `testdata/` 中 eastmoney fixtures；`sina_market*`、`sina_futures*` 保留。
- `internal/port`：删除 `MarketSource` 接口及 `FetchBars`/`FetchCorporateActions` 契约；`DailyMarketSource` 与其余 port 契约不动；`contracts_test.go` 的 `fakeMarketSource` 一并清理。
- `cmd/migrate-strategy-kernel`：main.go 删除 `broker.NewEastmoneyMarketSource()` 装配，不再 import `pkg/broker`。

## 2. 共享函数搬移

`classifyEastmoneyRequestError`、`parseEastmoneyScaled` 定义于被删文件、被 `sina_market.go` 引用：搬入 `broker.go` 并重命名为 `classifyUpstreamRequestError`、`parseScaledPrice`（纯重命名，无行为变化，调用点同步更新）。其余仅被 eastmoney 使用的常量与函数随文件删除。

## 3. 测试策略

- `legacy_migrator_test.go`：现有 `TestLegacyBackfillRequiresExactDatesAndAdjustmentCoverage` 等基于 fake source 的用例重写为旧表直迁语义——正常转换、重复日期、非法 OHLC、空 timeframe 各自的失败分类；因子/actions 断言为空。
- `cmd/migrate-strategy-kernel` 测试：装配断言更新（无外部源）。
- `pkg/broker`：eastmoney 测试删除后覆盖率不低于阈值；sina 测试不因重命名变化。

## 4. 实施顺序

1. 迁移器重构 + cmd 装配收缩 + 测试重写（commit 1）
2. 删除 eastmoney 文件、MarketSource 接口、共享函数搬移重命名（commit 2）
3. 文档同步与变更记录（commit 3）

每步可编译、测试全绿。

## 5. 回滚

代码回滚即恢复 Eastmoney 装配；已执行的 dry-run 不写库，apply 前无持久化影响。若 apply 后发现数据问题，迁移版本发布链路既有幂等/失败语义兜底。
