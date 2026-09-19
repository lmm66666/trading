---
id: CHG-2026-09-19-legacy-finance-macro-cleanup-VERIFICATION
result: pending
authority: evidence
---

# 验证记录

整体状态见 [requirements](requirements.md)，批准目标见 [design](design.md)。

## 追踪矩阵

| 需求 | 设计 | 实现/文档 | 检查 | 结果 |
|---|---|---|---|---|
| LFC-001 | §2.1/§2.2 | api handler 删除、business/financialscreen 整删 | `go test ./...` 通过（全包）；router.go 仅余 16 条 `/api/v1/*` 路由 + refresh，全仓 grep 无旧端点/已删包残留 | passed |
| LFC-002 | §1.1/§2.1 | market_refresh.go、router/handler 收缩 | market_refresh_test.go 8 个测试通过（202 触发/429/200 单证券/半身份 400/404/409/身份不匹配 404/身份校验 400/未配置 500） | passed |
| LFC-003 | §2.2 | broker 旧文件、pkg/indicator 删除；测试 limiter stub | `go vet ./...` 无告警；market_ingestion_service_test.go 通过（limiter stub 语义不变） | passed |
| LFC-004 | §2.3 | data/model 清单；runtimeModels 收缩 | 全仓 grep 无 financial_report/shibor/exchange_rate 引用；data_test.go `TestRuntimeModelsKeepOnlyLegacyMigrationInfoTable` 通过，mock 断言仅建 `t_stock_info` | passed |
| LFC-005 | §2.4 | DROP SQL（服务停止窗口） | `financial_reports` 存在；`shibors`/`exchange_rates` 实际不存在，`DROP TABLE IF EXISTS` 覆盖；待停服窗口执行后以 information_schema 确认 | pending |
| LFC-006 | §2.1 | log→slog（main.go 3 处 + import 删除） | 全仓 grep 标准库 `log` 调用零残留 | passed |
| LFC-007 | §3 | 文档同步与 owns 清单 | TestDocumentationContract/Links 通过；`npm --prefix web run check` 通过 | passed |
| 门禁 | §4.5 | verify.sh | `bash scripts/verify.sh` 全过：go build/test/vet + web check + 总覆盖率 89.0%（阈值 80%），核心包 market 94.3%、indicator 91.3%、strategy 94.8%、backtest 90.4%（阈值 90%）；`--mysql` 不适用：无内核持久化语义变化，无新增 SQL 逻辑；`--image` 不适用：无镜像/部署变化 | passed |

## 迁移链路 dry-run 联验（2026-09-19）

约束"迁移链路不得破坏"的运行时证据：`cmd/migrate-strategy-kernel -dry-run` 对本地库完整执行，读旧库正常（4979 只证券、5,187,881 日线、1,042,966 周线、LastLegacyIDs 正常），报告生成与退出码语义正确（数据不完整时输出 `legacy migration: incomplete market data` 并以 1 退出）。

全部 4979 只失败：4968 只 `SOURCE_UNAVAILABLE`、11 只 `INCOMPLETE_DATA`。人工复核确认 baidu/sina 直连 200 正常，`push2his.eastmoney.com` 连接与 TLS 握手成功后服务端直接空响应断开（模拟浏览器 UA/Referer/HTTP1.1 无效）——Eastmoney 对当前网络出口拒绝服务，属环境问题而非本变更代码缺陷（迁移器与 eastmoney_market.go 本变更未修改，`go test ./...` 含其全部测试通过）。待网络出口恢复后重跑 dry-run 核对统计，再决定 apply。
