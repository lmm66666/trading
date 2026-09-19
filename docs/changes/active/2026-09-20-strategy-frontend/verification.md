---
result: passed
---

# 前端策略功能验证记录

2026-09-20，在独立 worktree（`codex/strategy-frontend`）执行；代码基线为 main `c69c9fa` 加本变更四个提交。本变更仅改动 `web/` 与设计文档，无 Go 生产代码改动。

## 实际执行

- `npm --prefix web run check`（tsc + vitest + build）：通过，12 个测试文件 69 个用例全绿；前端语句覆盖率 89.93%。
- `go test ./...`：补齐本验证记录后通过（此前仅 `TestDocumentationContract` 因记录缺失红灯，属预期测试先行）。
- `go vet ./...`：通过，退出码 0。
- `bash scripts/verify.sh`：退出码 0，全部 10 步通过——前端测试/覆盖率/生产构建、文档契约、Go 全量测试（总覆盖率 88.7%）、核心领域（market 94.3%、indicator 91.3%、strategy 94.8%、backtest 90.4%）、Race Detector、go vet、全市场性能用例（full_scan≈224ms）、容器配置安全检查；MySQL 与镜像未选择（理由见门禁适用性）。

## 需求验收证据

| 需求 | 结果与证据 |
|---|---|
| SWF-001 | `StrategyForm.test.tsx`：目录渲染参数定义与默认值；目录加载失败错误态（目录请求 reject） |
| SWF-002 | `ScanPanel.test.tsx`：提交体断言（UUID mock、日期转 `T00:00:00Z`、exchanges/active_only/limit 默认值） |
| SWF-003 | `ScanPanel.test.tsx`/`BacktestPanel.test.tsx`（fake timers）：2s 轮询节奏、终态停止、取消调用、卸载停止、连续网络错误停止 |
| SWF-004 | `ScanPanel.test.tsx`：首屏快照 key 固定、`after_sequence` 续页、无 `next_sequence` 结束、failures 折叠、行点击回调 |
| SWF-005 | `BacktestPanel.test.tsx`：元→缩放整数换算（Math.round）、`lot_size` 默认当前证券、`hold_bars` 留空上送 0、参数 min/max 拦截提交 |
| SWF-006 | `BacktestPanel.test.tsx` + `EquityChart.test.tsx` + `OrdersTradesTables.test.tsx`：summary 渲染含空值"—"、金额 ÷10000 展示、equity 连续拉取至无 `next_sequence`、订单/成交游标续页 |
| SWF-007 | `chartData.test.ts` + `App.test.tsx`：`tab` 白名单 chart/scan/backtest、非法值回落 chart |
| SWF-008 | `BacktestPanel.test.tsx`：初始资金越界（>10 亿）拦截不出请求、费率越界拦截 |
| SWF-009 | `App.test.tsx`：localStorage 写入与恢复（`wb.scan_run_id`/`wb.backtest_run_id`） |
| SWF-010 | `ScanPanel.test.tsx`/`BacktestPanel.test.tsx`：409 `IDEMPOTENCY_CONFLICT` 提示重新提交、错误态保留已展示内容 |

## 门禁适用性

- 本变更不涉及持久化、策略内核、镜像或 Go 代码语义：`--mysql`、`--image`、`--full` 不触发；远端 MySQL 与镜像构建记录为 not-required，未执行，不计为通过。
- 设计与后端契约的两处差异（参数对象映射、快照行无 name）按用户 2026-09-20 裁决"按契约适配"，已登记于 design.md 第 7 节。
