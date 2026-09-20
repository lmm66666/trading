---
id: CHG-2026-09-21-board-persistence-VERIFICATION
result: blocked
authority: evidence
---

# 验证

需求与生命周期见 [requirements](requirements.md)，目标见 [design](design.md)。

`result` 为 pending/passed/failed/blocked，不复制变更生命周期状态。

## 追溯矩阵

| 需求 | 设计章节 | 实现 | 测试 | 结果 |
|---|---|---|---|---|
| PERSIST-001 服务端存储 | 数据模型与存储 | `internal/infrastructure/mysql/chart_board_model.go`（`t_chart_boards`，`migrationModels` 注册于 `migrate.go`） | `mysql/chart_board_store_integration_test.go`（HasTable、CRUD、激活迁移、删除激活行、末板保护）；`application/chart_board_service_test.go` | passed |
| PERSIST-002 看板 API | HTTP 契约 | `api/list_chart_boards.go`、`create_chart_board.go`、`update_chart_board.go`、`activate_chart_board.go`、`delete_chart_board.go`、`router.go` 五路由、`main.go` 装配 | `api/chart_boards_test.go`（信封透传、strictJSON 含未知字段与尾随 JSON、错误映射 400/404/409、未装配 500 脱敏、非数字 id 400） | passed |
| PERSIST-003 服务端校验 | 端口与应用层 | `internal/port/chart_board.go`、`internal/application/chart_board_service.go`（名称、config 严格解析、指标复用图表口径、数量上限、末板保护） | `application/chart_board_service_test.go`（20 个 config 校验 case、名称 trim 与长度、满 20 拒绝、Update 变体、NotFound 传播、规范化序列化） | passed |
| PERSIST-004 激活看板 | 存储事务 | `mysql/chart_board_store.go`（Create 插入激活行、Activate 双向条件更新、Delete 后激活 id 最小者，全部事务包裹） | `mysql/chart_board_store_integration_test.go`（创建即激活、重复激活幂等、删除激活行 min-id 接替、删到空表） | passed |
| PERSIST-005 前端改造 | 前端改造 | `web/src/api/client.ts`（五函数与 DTO）、`boards.ts`（删 localStorage、保留预检）、`useBoards.ts`（异步 API 化）、`BoardToolbar.tsx`、`ChartWorkspace.tsx`、`App.tsx` | `boards.test.ts`（预检口径）、`useBoards.test.tsx`（加载/空表首建/保存/切换两步/失败保留草稿/busy/初始股票回调/末板）、`BoardToolbar.test.tsx`（保存/重命名/另存/删除/切换对话框/键盘焦点）、`ChartWorkspace.test.tsx`（查询、分页、看板保存恢复）、`App.test.tsx`（URL 优先、看板恢复默认股票、加载失败重试、空表建默认看板） | passed |
| PERSIST-006 并发语义修订 | 并发、容量与安全 | `useBoards.ts`（操作级最后写入、响应全量替换本地）+ 服务端 `is_active` 事务不变量 | `useBoards.test.tsx`（失败保留草稿与本地状态）；`mysql/chart_board_store_integration_test.go`（同值更新不误报 NotFound） | passed |

## 已执行检查

- `npm --prefix web run check`（vitest 137 通过 + tsc + vite build）：通过。
- `go test ./...`：全部 ok。
- `bash scripts/verify.sh --mysql`（2026-09-21，worktree `codex/board-persistence`）：[1/10]–[8/10] 全部通过；[9/10] `TestDeploymentMySQLCompatibility` 失败——远端 MySQL 服务器（192.168.31.85:45709）不可达（TCP ping 与 ICMP 均不通，服务器离线），属环境问题而非代码缺陷，按工程标准准确记录为未完成；待环境恢复后重跑 `--mysql` 补齐。[10/10] `--image` 未包含在本次运行（`--mysql` 变体）。
- `--image` 门禁：not-required——无构建、依赖或部署变化（无 Dockerfile/.dockerignore/依赖清单改动），按设计记录为不适用。

## 覆盖率

verify.sh [3/10] 与 [4/10] 输出（2026-09-21）：

- 总计覆盖率：86.9%（门槛 80%）。
- market 94.3%、indicator 91.2%、strategy 94.8%（strategy 95.2% + builtin 94.1%）、backtest 90.4%（核心门槛均 90%）。
- 其余关键包：application 89.5%、port 93.9%、infrastructure/mysql 83.9%。

## 偏差与剩余风险

1. **useBoards 挂载位置**：设计表述"useBoards 仍挂载于 ChartWorkspace"，实现提升到 `App.tsx`。原因：ChartWorkspace 仅在已选证券时挂载，URL 无股票且看板有 defaultSymbol 时 GET 永不触发，默认股票无法恢复（设计自身的"加载回调上抛"依赖看板先加载）。语义与设计其余条款完全一致（App 不重复请求、单数据源）。
2. **Update 存储实现**：Update/Activate 采用"先 `Take` 存在性检查再无条件更新"，而非设计所述"单条条件 UPDATE（affected=0 → NotFound）"。原因：MySQL 默认 client flag 无 CLIENT_FOUND_ROWS，同值 UPDATE 的 RowsAffected=0 会把已存在行误判为 NotFound。集成测试覆盖"同值更新不误报"。
3. 前端 `App.tsx` 错误横幅在 `board.status === 'error'` 时渲染（设计"加载失败显示错误与重试"的落地形式）。

## 证据版本与阻塞

- 测试基线：worktree `codex/board-persistence`（CHG-2026-09-21-board-persistence 实现分支）。
- 独立子 Agent 评审：已通过（2026-09-21）——架构、遗留清理、简化无发现；缺陷类仅 3 处 note（均为设计已声明接受的风险或不可达路径），结论"可归档合并（除 --mysql 门禁待环境恢复补跑外）"。
- 阻塞事项：远端 MySQL 服务器（192.168.31.85:45709）离线，`--mysql` 门禁（deployment 兼容 + integration 测试）无法执行。
- 用户裁决（2026-09-21）：接受本地门禁 9/10 通过的现状，代码先合并回 main；`--mysql` 待环境恢复后补跑，通过后将本变更 status 改为 implemented 并移入 `archive/`。
