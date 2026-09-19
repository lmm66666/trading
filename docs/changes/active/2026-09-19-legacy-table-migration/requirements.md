---
id: CHG-2026-09-19-legacy-table-migration
status: implementing
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-19"
approved_revision: "de5a212:docs/changes/active/2026-09-19-legacy-table-migration/"
approved_scope: [LTM-001, LTM-002, LTM-003, LTM-004, LTM-005]
---

# 迁移数据源收敛为旧表直迁

| 属性 | 内容 |
|---|---|
| 创建日期 | 2026-09-19 |
| 适用范围 | internal/infrastructure/mysql 迁移器、cmd/migrate-strategy-kernel、pkg/broker、internal/port、相关设计文档 |
| 基线 | de5a212（main） |
| 已确认意图 | 用户于 2026-09-19 会话裁决：Eastmoney 上游对当前网络出口拒绝服务且不可预期恢复；"不要用 eastmoney，目前只有一个数据源就是 sina"；迁移改为旧表直迁（方案 B），QFQ 因子由激活后的全市场刷新（新浪 qfq.js，全量断点 + 首断点回拨）补齐 |
| 批准证据 | 用户 2026-09-19 会话裁决："不要用 eastmoney，目前只有一个数据源就是 sina"（选择旧表直迁）；"按原设计删"（确认彻底删除 Eastmoney）；"行，那就按照计划执行吧"（批准本变更按 3 commit 实施） |

## 1. 问题、目标与使用条件

迁移器当前对每只证券调用 `port.MarketSource`（Eastmoney）重新拉取 raw 行情与复权因子并做日期对齐校验。Eastmoney 从本网络出口已无法访问（连接与 TLS 握手成功后被服务端空响应断开，baidu/sina 正常），迁移 dry-run 全部 4979 只证券失败，迁移链路被外部依赖阻塞。

已核实的事实链：

- 旧表 `t_stock_kline_daily` 数据与新浪 `getKLineData` 接口逐分一致（601288 在 2026-05-13 除权日附近抽样比对），旧表即新浪 raw 日线，不含复权因子与公司行动。
- 生产刷新（新浪）每次拉全量 `qfq.js` 事件式断点因子，并把首断点回拨到合并后全历史首根 bar（`market_ingestion_service.go` L255-259）；首次全市场刷新后 QFQ 全历史可用。
- 新浪无公司行动明细；现有 RAW/QFQ 视图、指标、策略、回测均不消费公司行动。

## 2. 稳定需求

- LTM-001：迁移器改为旧表直迁：日线与周线 bars 直接从旧表行转换迁入（价格缩放 10000、`NewDataset` 校验兜底），复权因子与公司行动为空列表，移除上游回填调用与因子覆盖校验；锁、检查点、报告、发布与幂等语义不变。
- LTM-002：删除 `EastmoneyMarketSource` 及其测试与 fixtures；`port.MarketSource` 接口随唯一实现与唯一消费者一并删除（`DailyMarketSource` 保留）；迁移命令不再装配任何外部行情源。
- LTM-003：被 sina_market.go 引用的共享函数（`classifyEastmoneyRequestError`、`parseEastmoneyScaled` 等）搬移并重命名为上游通用名，无行为变化。
- LTM-004：文档同步：`cmd/migrate-strategy-kernel.md` 设计（数据源语义、回填不变量）、`pkg/broker.md` owns 清单、`docs/design/README.md` 源码地图、`docs/roadmap.md` 兼容面描述（Eastmoney 链路从剩余兼容面移除）。
- LTM-005：dry-run 全量验收：本地库完整跑通、无 RejectedCodes/Failures、报告质量 COMPLETE；apply 与激活 SQL 在用户停服窗口执行（另行协调，不阻塞本变更代码验收）。

## 3. 非目标

- 不删除旧表、`data`/`model` 兼容模型与迁移命令本身（迁移验收后由后续变更处理）。
- 不修改生产刷新链路（新浪源、因子拉取、首断点回拨语义已满足需求）。
- 不补公司行动数据源；未来股息现金流类功能需要时另行立项。
- 不改变 QFQ 读路径"因子缺失即拒绝"的既有行为。

## 4. 方案比较与选择

选择：旧表直迁 + 激活后刷新补因子（方案 B）。

放弃新浪迁移适配器（方案 A）：迁移即带全量因子，但需约 1 万次上游请求（限频下约 3 小时），引入新浪批量限流风险且中断需全量重跑；其收益（QFQ 提前一晚可用）不值得。放弃等待 Eastmoney 恢复：不可预期，阻塞迁移验收。

## 5. 可执行验收

| 需求 | 验证 |
|---|---|
| LTM-001 | 迁移器单测：旧表行→Bar 转换、因子/actions 空、非法 OHLC/重复日期报 INCOMPLETE/INVALID；`go test ./...` |
| LTM-002 | 全仓 grep 无 `EastmoneyMarketSource`/`port.MarketSource` 残留；编译通过 |
| LTM-003 | `go vet ./...`；broker 测试全过 |
| LTM-004 | `TestDocumentation*`；人工核对设计文档差异 |
| LTM-005 | dry-run 完整输出：4979 只、无 Failures、Quality COMPLETE |
| 门禁 | `npm --prefix web run check && go test ./... && go vet ./... && bash scripts/verify.sh`；--mysql/--image 不适用（无内核持久化语义与部署变化） |

## 6. 风险

- 激活到首次全市场刷新完成之间（约一晚）QFQ 不可用，用 QFQ 的扫描/回测失败；RAW 正常。用户已知并接受（个人项目，激活后立即触发刷新）。
- qfq.js 为空的股票（从未分红/次新股）生产刷新既有语义即报因子不完整，此类证券 QFQ 持续缺失；属既有行为，非本变更引入。
- 旧表周线直迁失去原 Eastmoney 日期对齐校验，由 `NewDataset` 固有校验（升序、无重复、OHLC 合法）兜底；旧表数据与新浪逐分一致已抽样验证，可信。
