---
id: CHG-2026-09-19-legacy-finance-macro-cleanup-DESIGN
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-19"
approved_revision: "d30c051:docs/changes/active/2026-09-19-legacy-finance-macro-cleanup/design.md"
approved_scope: [LFC-001, LFC-002, LFC-003, LFC-004, LFC-005, LFC-006, LFC-007]
owns: []
related: ["docs/changes/active/2026-09-19-legacy-finance-macro-cleanup/requirements.md"]
---

# 目标设计：下线财报/宏观兼容能力与旧 URL 层

| 属性 | 内容 |
|---|---|
| 创建日期 | 2026-09-19 |
| 适用范围 | 同需求文档 |
| 上游 | requirements.md（LFC-001..007） |

## 1. 合并后状态

### 1.1 HTTP 面

路由表仅保留 `/api/v1/*` 内核端点，新增一个手动刷新端点：

```text
POST /api/v1/market/refresh
  请求体（可选）: {"exchange": "SSE", "code": "600000"}
    - exchange 与 code 必须同时提供或同时缺省；任一单独出现为 400。
    - 提供 instrument：经 Instruments.ResolveCode 解析（唯一匹配），
      同步调用 MarketIngestion.Refresh，200 返回刷新结果。
      解析零结果映射 404，多值映射 409（沿用既有 errAmbiguousInstrument）。
    - 不提供：调用 MarketTrigger.TriggerNow(MarketWorkers)，返回
      202 Accepted {"status":"ACCEPTED"}，语义与原 /api/stocks/append 一致。
  错误：JSON 解析失败/kernel 未配置沿用既有 writeApplicationError 分类。
```

删除的 11 个端点：`POST /api/stocks/financial-report`、`POST /api/stocks/financial-report/append`、`GET /api/stocks/financial-report`、`GET /api/stocks/financial-report/signal`、`GET /api/macro/shibor`、`GET /api/macro/exchange-rate`、`GET /api/stocks/price`、`GET /api/stocks/backtest`、`GET /api/stocks/signal`、`POST /api/stocks/historical`、`POST /api/stocks/append`。

### 1.2 装配与包结构

- `NewRouter(kernel KernelServices) *gin.Engine`：单参数；`/api/v1/*` 路由不变，追加 refresh 路由。
- `StockHandler` 收敛为仅含 `kernel KernelServices` 的结构（文件归属 handler.go；`NewStockHandler` 删除，路由直接使用 `KernelServices` 方法接收者或保留一个薄构造器，实现时取最简形式）。
- `main.go`：删除 `NewSinaBroker`/`NewEastMoneyBroker`、financialSvc/financialScheduler/signalSvc/querySvc/macroSvc 装配与 scheduler 生命周期；`run()` 中 `data.New` 之后直接 `newKernel` + `api.NewRouter(kernel.services)`。
- 模块依赖变化：`api` 不再依赖 `business`；`business`、`financialscreen`、`pkg/indicator` 包消失；`pkg/broker` 仅剩新浪行情/期货适配器与东财迁移适配器（后者归后续变更删除）。

## 2. 删除清单

### 2.1 api（13 文件 + 2 收缩）

删除 handler 及其测试：`append_financial_report_data.go(+test)`、`get_exchange_rate.go(+test)`、`get_financial_report.go(+test)`、`get_financial_report_signal.go(+test)`、`get_shibor.go(+test)`、`get_stock_backtest.go(+test)`、`get_stock_buy_signals.go(+test)`、`save_financial_report_data.go(+test)`、`append_stock_data.go`、`get_stock_price.go`、`save_stock_historical_data.go`（后三者无独立测试文件）。

删除整文件测试：`market_cutover_test.go`、`kernel_boundaries_test.go`（断言对象为旧 URL→内核映射）。

收缩：`router.go`（路由与签名）、`handler.go`（StockHandler 字段、`respondInternalError` 改 slog）。辅助函数归属（已核实引用）：`positiveQueryInt` 定义于待删的 `get_stock_price.go`、被保留的 `get_market_bars.go` 引用，随删除迁移至 `get_market_bars.go`；`timeframeName` 定义于保留的 `strategy_handler.go`，不动。

新增：`market_refresh.go` + `market_refresh_test.go`。

### 2.2 business / financialscreen / pkg

- `business/`：整目录（15 文件）。
- `internal/financialscreen/`：整目录（含测试）。
- `pkg/broker/`：`sina.go`、`sina_test.go`、`eastmoney.go`、`eastmoney_test.go`、`broker.go`。保留 `sina_market*`、`sina_futures*`、`eastmoney_market*`（迁移依赖）、`testdata/` 中 `eastmoney_*`、`sina_futures_daily.js`。
- `pkg/indicator/`：整目录（8 文件）。`internal/application/market_ingestion_service_test.go` 的 `pkg/indicator` 引用改为测试内并发 limiter stub（`UpstreamLimiter` 为接口，无需生产改动）。

### 2.3 data / model / shell

- `data/`：删除 `financial_report.go`、`stock_info.go`、`stock_kline_daily.go`、`stock_kline_weekly.go`；`data.go` 删除全部 repo getter，`runtimeModels()` 收缩为 `[&model.StockInfo{}]`；`data_test.go` 同步。
- `model/`：删除 `financial_report.go`、`shibor.go`、`exchange_rate.go`；保留 `stock_info.go`、`stock_kline*.go`（迁移链路依赖）。
- `shell/`：整目录删除（`save_financial_report.sh`、`save_stock_historical.sh`、`code/*.txt`、`__pycache__/`）。

### 2.4 数据库

```sql
DROP TABLE IF EXISTS financial_reports, shibors, exchange_rates;
```

`FinancialReport` 移出 `runtimeModels` 后重启不重建。`t_stock_info`、`t_stock_kline_daily`、`t_stock_kline_weekly` 本变更不触碰。

## 3. 文档同步

| 文档 | 动作 |
|---|---|
| `docs/design/business.md`、`docs/design/internal/financialscreen.md` | 删除文件 |
| `docs/design/README.md` | 模块地图删 business、financialscreen、pkg/indicator 行；源码树更新；"当前兼容依赖"收缩为 `data -> config/MySQL adapter` 与 `pkg/broker -> model`（迁移适配器）|
| `docs/design/api.md` | 移除 11 端点，新增 refresh 契约 |
| `docs/standards/http-api.md` | 同步端点契约与示例 |
| `docs/design/pkg/broker.md` | owns 收缩；删除 §2 旧能力、§5.1 旧 `SinaBroker` 兼容节及对应测试描述 |
| `docs/design/data.md` | 删除财报/主数据/旧 K 线 repo 章节，仅留连接初始化与 StockInfo 迁移兼容说明 |
| `docs/architecture/system-design.md` | 兼容链路描述更新 |
| `docs/roadmap.md` | "兼容边界收敛"标注财报/宏观部分已落地，迁移链路部分指向后续变更 |
| `documentation_test.go` | owns 清单移除已删文档与路径 |
| `config.example.yaml` | 无兼容配置项，预期无变化；如有财报/宏观残留项一并清理 |

## 4. 实施顺序

1. api 层：删 handler/路由 → 改 NewRouter/StockHandler → 新增 refresh 端点及测试。
2. 依赖层：删 business、financialscreen、pkg/indicator、pkg/broker 旧文件、data/model 清单。
3. 组合根：main.go 装配收缩 + log→slog。
4. 文档与 owns 同步、shell 删除。
5. 门禁：`go test ./... && go vet ./... && npm --prefix web run check && bash scripts/verify.sh`。
6. DB DROP（服务停止窗口执行，作为验收证据记录）。

Commit 划分：① api 端点收敛与新端点；② business/financialscreen/broker/indicator/data/model 删除与 main.go 收缩；③ 文档与 shell 同步；④ DROP 记录（变更记录内回填 result，不单独 commit SQL）。

## 5. 测试策略

- 新增 `market_refresh_test.go`：带 instrument 200（stub Ingestion）、不带 202（stub Trigger）、半身份 400、解析 404/409、kernel 未配置 503。
- 既有 `router_test`/`handler_test` 中涉及旧端点的用例删除；`serve_web`、`strategy_handler`、`kernel_handler` 等保留用例不动。
- `market_ingestion_service_test.go`：limiter stub 替换后语义不变（仍验证 Limiter 钩子的获取/释放与取消传播）。
- 无 SQL mock 需求；MySQL 集成门禁不触发（无持久化语义变化），记录不适用理由。

## 6. 回滚

单 revert 即可恢复代码与文档；DROP 的三表数据按用户裁决不保留，不做数据回滚预案。
