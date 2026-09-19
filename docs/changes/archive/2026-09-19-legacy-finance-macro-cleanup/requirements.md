---
id: CHG-2026-09-19-legacy-finance-macro-cleanup
status: implemented
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-19"
approved_revision: "d30c051:docs/changes/active/2026-09-19-legacy-finance-macro-cleanup/"
approved_scope: [LFC-001, LFC-002, LFC-003, LFC-004, LFC-005, LFC-006, LFC-007]
---

# 下线财报/宏观兼容能力与旧 URL 层

| 属性 | 内容 |
|---|---|
| 创建日期 | 2026-09-19 |
| 适用范围 | api、business、internal/financialscreen、pkg/broker、pkg/indicator、data、model、shell、main.go、相关设计文档与 HTTP 契约 |
| 基线 | d30c051（main，工作区仅存在与本变更无关的 docs/analysis 删除） |
| 已确认意图 | 用户于 2026-09-19 会话裁决：本项目为个人实验项目，财报、宏观（SHIBOR/汇率）与旧实时行情能力不保留，相关代码清理干净；旧 URL 兼容层一并删除，"保持代码简洁干净，不要没用的兼容层"；旧数据表直接 DROP（决策 B）；两个手动刷新端点合并为一个 v1 端点；data 包在后续变更中收敛 |
| 批准证据 | 用户于 2026-09-19 批准需求与目标设计（"网络已经修复，mysql 也已经装好，按照计划实施吧"），范围 LFC-001..007 及 design.md 全部删除清单与 refresh 端点契约 |

## 1. 问题、目标与使用条件

旧业务栈（`api → business → data/model → SinaBroker/EastMoneyBroker`）仍在生产装配中运行财报、宏观与旧 URL 端点，但用户已裁决这些能力不再需要。前端 `web/` 对 `/api/stocks/*`、`/api/macro/*` 零调用；旧 URL 中 price/backtest/signal 已是内核薄适配，仅 URL 风格遗留。旧行情到版本化内核的迁移尚未执行（另行处理），迁移链路依赖 `model.StockInfo` 与 `model.StockKline`，本变更不得破坏。

本变更是纯删除与收敛：除一个手动刷新 v1 端点外不新增业务能力，不改变内核行为。

## 2. 稳定需求

- LFC-001：下线 6 个财报/宏观 HTTP 端点（`POST/GET /api/stocks/financial-report*`、`GET /api/stocks/financial-report/signal`、`GET /api/macro/shibor`、`GET /api/macro/exchange-rate`），删除 `business` 整包与 `internal/financialscreen` 整包。
- LFC-002：删除 5 个旧 URL 端点（`/api/stocks/price|backtest|signal|historical`、`/api/stocks/append`），新增 `POST /api/v1/market/refresh` 合并两个手动刷新能力；`NewRouter` 收敛为单一 `KernelServices` 参数，`StockHandler` 旧业务字段全部移除。
- LFC-003：删除失去调用方的旧适配器 `SinaBroker`、`EastMoneyBroker` 与 `Broker` 接口；删除 `pkg/indicator` 整包（旧指标为 business 孤儿，并发 Limiter 由 application 测试改用测试内 stub）。保留 `SinaMarketSource`、`SinaFuturesSource` 与共享限频装配。
- LFC-004：删除 `data` 包中财报、股票信息、旧日/周 K 线 repo 及 `Data` 上的对应 getter；`runtimeModels()` 仅保留 `model.StockInfo`（迁移器 stage "info" 依赖 `t_stock_info`，禁止在本变更删除）。删除 `model` 包中 `FinancialReport`、`Shibor`、`ExchangeRate`；保留 `StockInfo`、`StockKline*`（迁移依赖，属后续迁移链路删除变更）。
- LFC-005：DROP 数据库表 `financial_reports`、`shibors`、`exchange_rates`；服务重启后 AutoMigrate 不得重建这些表。
- LFC-006：剩余标准库日志（main.go 3 处、api `respondInternalError` 1 处）迁移到 `slog`。
- LFC-007：同步全部受影响权威文档与门禁：删除 `business.md`、`financialscreen.md`；更新模块地图（README）、api 设计、HTTP 契约、broker 设计、data 设计、系统设计、roadmap 与 `documentation_test.go` 的 owns 清单；`shell/` 目录整体删除。

## 3. 非目标

- 不删除、不修改旧行情迁移链路：`cmd/migrate-strategy-kernel`、`EastmoneyMarketSource`、`port.MarketSource`、MySQL `LegacyMigrator` 及 `model/stock_kline*`、`model/stock_info`、表 `t_stock_info`/`t_stock_kline_daily`/`t_stock_kline_weekly`——迁移验收后由后续变更处理。
- 不收敛 `data.New` 连接初始化（data 包整体收敛属后续变更）。
- 不改变内核行情、指标、策略、回测、扫描行为；不新增盘中实时行情。
- 不做依赖升级或无关重构。

## 4. 方案比较与选择

选择：单变更内完成能力下线 + 死代码删除 + 文档同步，端点合并为 `POST /api/v1/market/refresh`。

放弃仅注释路由保留代码：违反"不要没用的兼容层"裁决，且 `business`/旧 broker 将长期成为无调用的维护面。放弃逐端点分批下线：个人实验项目无外部消费者（前端零调用），分批只增加中间态验证成本。

## 5. 可执行验收

| 需求 | 验证 |
|---|---|
| LFC-001/002 | `go test ./...` 中旧端点与旧行为测试全部移除；路由表仅剩 `/api/v1/*`；`POST /api/v1/market/refresh` 带 instrument 200、不带 202 的契约测试 |
| LFC-003 | `pkg/broker`、`pkg/indicator` 编译无旧类型引用；`market_ingestion_service_test.go` 使用测试内 limiter stub 通过 |
| LFC-004 | `go vet ./...`；全仓 grep 无 `FinancialReport`/`Shibor`/`ExchangeRate` 生产引用；`runtimeModels` 仅含 `StockInfo` |
| LFC-005 | DROP SQL 执行记录；重启服务后 `information_schema` 确认三表不存在 |
| LFC-006 | 全仓生产代码无标准库 `log` 调用 |
| LFC-007 | `go test ./... -run TestDocumentation`（owns/链接契约）；`npm --prefix web run check`；人工核对模块地图与 HTTP 契约差异 |
| 门禁 | `go test ./... && go vet ./... && npm --prefix web run check && bash scripts/verify.sh`；--mysql/--image 未触发（无内核与持久化语义变化），记录为不适用 |

## 6. 风险

- 删除面大但均为已验证的零调用方代码（前端 grep 零匹配、调用图核查见 design.md）；主要风险是遗漏隐式引用，由编译与全仓 grep 门禁兜底。
- DROP 不可逆：项目裁决为个人实验项目且数据无保留价值，接受。
- `POST /api/stocks/historical` 的 curl 灌数脚本随端点删除；替代路径为激活后调度器全量刷新 + 新端点单证券刷新，已在设计中确认等价覆盖。
