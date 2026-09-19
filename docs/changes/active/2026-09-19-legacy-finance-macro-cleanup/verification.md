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
| LFC-001 | §2.1/§2.2 | api handler 删除、business/financialscreen 整删 | `go test ./...`；路由表 grep | pending |
| LFC-002 | §1.1/§2.1 | market_refresh.go、router/handler 收缩 | market_refresh_test.go | pending |
| LFC-003 | §2.2 | broker 旧文件、pkg/indicator 删除；测试 limiter stub | `go vet ./...`；market_ingestion_service_test.go | pending |
| LFC-004 | §2.3 | data/model 清单；runtimeModels 收缩 | 全仓 grep；data_test.go | pending |
| LFC-005 | §2.4 | DROP SQL（服务停止窗口） | information_schema 确认 | pending |
| LFC-006 | §2.1 | log→slog 4 处 | 全仓 grep 标准库 log | pending |
| LFC-007 | §3 | 文档同步与 owns 清单 | TestDocumentation*；npm check | pending |
| 门禁 | §4.5 | verify.sh | 完整输出记录 | pending |
