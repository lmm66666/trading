---
id: CHG-2026-09-21-scan-workbench-redesign-verification
result: passed
authority: evidence
---

# 扫描工作台重设计验证记录

变更状态由 [需求](requirements.md) 管理，目标见 [设计](design.md)。本记录的 passed 只表示用户调整后的交付范围通过，不表示远端 x86_64 MySQL 验收已执行。

## 验收范围与用户决定

`bash scripts/verify.sh --mysql` 第 9 步要求远端 MySQL 8.4/x86_64：本机 `config.yaml` 指向 `127.0.0.1:3306`（MySQL 8.4.11，架构 arm64），部署兼容性测试与全部集成测试在 arm64 上拒绝运行；获批验收目标 NAS（x86_64）当日不可达。用户于 2026-09-21 明确批准**豁免本次远端 x86_64 MySQL 验收**（同 2026-09-21 NAS 服务拆分的处置），以本地门禁 1–8 与 sqlmock 单测作为本次交付门槛；后续 NAS 可达时仍需按工程标准补验。本次变更未改 Dockerfile/部署配置，`--image` 门禁不适用。

## 需求追踪

| 需求 | 实现 | 证据与结果 |
|---|---|---|
| REQ-SCAN-UI-001 | web/src/features/strategy/RunMonitor.tsx | RunMonitor.test.tsx 7 个测试通过：状态条仅徽标+策略 ID/版本，run_id 默认不可见，「任务详情」展开后可见 run_id/数据版本/尝试次数/取消标记 |
| REQ-SCAN-UI-002 | web/src/features/scan/ScanResults.tsx、ScanPanel.tsx | ScanPanel.test.tsx：终态合并单张结果卡且状态条唯一；表格列代码/名称/信号时间/信号原因；点击证券跳转图表视图回归通过 |
| REQ-SCAN-UI-003 | ScanResults.tsx | 空态「无入选证券」+调整建议且无空表头；加载中骨架行（role=status）测试通过 |
| REQ-SCAN-UI-004 | internal/port/signal_snapshot.go、run_store.go、internal/infrastructure/mysql/signal_snapshot_store.go、api/get_latest_signal_snapshot.go、web/src/api/client.ts | 后端 sqlmock 单测与 handler 契约测试通过（名称 join、失败行批量解析降级、name 条件输出）；前端名称缺失回退证券代码测试通过；远端 x86_64 隔离库验收按用户决定豁免 |
| REQ-SCAN-UI-005 | web/src/features/strategy/StrategyForm.tsx | 配置栏「策略」标签唯一、选择器保留可访问名、元信息 meta-chip 测试通过；参数 label 保持英文参数名 |
| REQ-SCAN-UI-006 | 同上 | 请求字段/校验/分页/状态机未改；BacktestPanel 既有测试回归通过 |

## 实际执行

- 基线与批准：设计批准版本 `4c2217b`（2026-09-21T17:07:35+08:00）；后端实现提交 `434babb`；前端测试先行，三个测试文件首跑 12 failed/24 passed（预期红灯），实现后 36 个测试全绿。
- `npm run check`（vitest 全量 + 覆盖率 + tsc + vite build）：通过，退出码 0。
- `bash scripts/verify.sh --mysql`：
  - [1/10] 前端测试、覆盖率与生产构建：通过。
  - [2/10] 文档契约：通过（首次运行因缺 verification.md 失败，补建后通过）。
  - [3/10] 全量测试与总覆盖率：通过，总计 86.6%（门槛 80%）。
  - [4/10] 核心领域覆盖率：market 94.3%、indicator 92.5%、strategy 94.8%、backtest 90.4%（门槛各 90%）。
  - [5/10] Race Detector：通过。
  - [6/10] 静态检查 `go vet ./...`：通过。
  - [7/10] 全市场性能门禁：通过（full_scan=120.9ms）。
  - [8/10] 容器配置安全检查：通过。
  - [9/10] 远端 MySQL：`TestDeploymentMySQLCompatibility` 连接本机 MySQL 8.4.11 成功但架构 arm64 非 x86_64，失败；`go test -tags=integration ./internal/infrastructure/mysql/... ./data` 同样在 arm64 守卫处失败。按用户上述决定豁免，不计为通过。
  - [10/10] 镜像：不适用（未改构建/部署配置）。
- 环境修补记录：worktree 缺 `config.yaml`（gitignore 不提交），从主仓库复制到 worktree 本地使用，未提交。

## 独立评审与修复

独立子 Agent 对 `main...HEAD` 全量 diff 只读评审（架构/遗留/简化/缺陷/测试），结论为代码无 P1 缺陷、需修复文档项后合并。已修复：

- P1-1：`docs/standards/http-api.md` 未同步 `name` 字段——已补 rows/failures 可选 `name`、读取时点显示属性语义与 rows 关联 `t_instruments` 的如实描述。
- P2-1：设计回填——`mysql.md` 补 Latest 读取的名称关联与失败行降级语义；`strategy-scan.md` 补结果行展示语义；`application.md` 未约定读取投影内容，按批准设计不改动。
- P2-2：本验证记录随分支提交。
- P3 建议同步修复：首屏加载期间结果卡标题显示「结果加载中…」而非「入选 0 只」；failures 表头「代码」改为「失败代码」与结果表区分。修复后 ScanPanel 22 个测试与文档契约重跑通过。

## 未验证范围

远端 MySQL 8.4/x86_64 的部署兼容性与随机隔离库集成测试未执行（本机 arm64、NAS 不可达，用户批准豁免）。名称关联 SQL 已由 sqlmock 单测覆盖，但未经真实 x86_64 MySQL 验证；NAS 可达后应重跑 `bash scripts/verify.sh --mysql` 补验。
