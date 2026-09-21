---
id: CHG-2026-09-21-DELAYED-MARKET-REFRESH-VERIFICATION
result: passed
authority: evidence
---

# 验证

需求见 [需求](requirements.md)，目标见 [设计](design.md)。被验证源码版本为 `20fb981382152f4f274df333b925b005973f065c`，后续仅整理业务说明、审批状态与本记录。

## 需求追踪

| 需求 | 实现 | 测试 | 结果 |
|---|---|---|---|
| REQ-DELAY-001 | 两类 Scheduler.Start 先等待 ticker | TestSchedulersWaitForFirstIntervalAndKeepTicking | passed |
| REQ-DELAY-002 | 原股票 TriggerNow 保持独立 | TestStockManualRefreshDuringInitialWaitDoesNotResetSchedule | passed |
| REQ-DELAY-003 | 原取消/守卫/错误分支 | TestSchedulersCancelBeforeFirstIntervalWithoutRefresh、既有生命周期与并发测试 | passed |

## 执行证据

- 新测试在旧实现下全部失败，明确观测到启动即采集；调整两个 Start 循环顺序后通过。
- `go test ./internal/application ./api -count=1`：通过。
- 调度聚焦 Race 测试：通过；独立 reviewer 重复 20 次亦通过。
- `bash scripts/verify.sh`：完整本地门禁通过，含前端、文档、Go 全量、覆盖率、全量 Race、静态、性能和容器配置检查。前端 162 个测试通过、语句覆盖率 91.81%；Go 总覆盖率 86.7%，market 94.3%、indicator 91.2%、strategy 94.8%、backtest 90.4%。
- `go build ./...` 与 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/trading-delayed-refresh-amd64 .`：通过。
- `git diff --check`：通过。
- 远端 origin fetch 成功，实施基于本地 main `20214a8`。原工作区界面修改不纳入本变更。

## 评审与文档

独立 review_delayed_refresh 未发现 P1/P2，确认取消、固定节拍、手动刷新和测试行为。指出业务说明中旧启动描述后已同步更新 `docs/design/workflows/market-data.md` 及其代码基线；应用层、系统设计、操作手册和 AGENTS 同步完成。

## 外部门禁

MySQL 与镜像构建均为 not-required：本变更仅调整进程内首次调度时机，不改变数据访问、持久化、接口、构建或部署方式。未访问 NAS，不将此前服务拆分中尚未执行的 NAS 部署验证声明为通过。
