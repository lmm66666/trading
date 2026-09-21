---
id: CHG-2026-09-21-NAS-SERVICE-SPLIT-VERIFICATION
result: pending
authority: evidence
---

# NAS 服务拆分验证记录

变更状态由 [需求](requirements.md) 管理，目标见 [设计](design.md)。当前仅整理设计，生产代码未改动。

## 需求追踪

| 需求 | 设计章节 | 计划验证 | 结果 |
|---|---|---|---|
| REQ-SPLIT-001/002 | 1、4、5 | 角色解析、装配隔离、无 all 模式 | pending |
| REQ-SPLIT-003 | 1、3 | 原采集范围、调度与限频回归 | pending |
| REQ-SPLIT-004 | 2、5 | 工作台 API、Worker、版本与分页回归 | pending |
| REQ-SPLIT-005/006 | 3、5 | 刷新双服务 HTTP、取消、错误映射、独立生命周期 | pending |
| REQ-SPLIT-007/008 | 2、6 | 隔离 MySQL 的初始化/纯连接与并发读写 | pending |
| REQ-SPLIT-009 | 3、4、5 | Token、固定 URL、大小限制、两个 amd64 镜像 | pending |
| REQ-SPLIT-010 | 6、8 | 完整门禁、覆盖率、独立评审、部署演练 | pending |

## 已执行检查

- 基线：`1975cbd06cb90b1f9e3fc4b363189f89668fd09a`。
- 独立 worktree：`.worktrees/nas-service-split`，分支 `codex/nas-service-split`。
- `git fetch github`：失败，SSH publickey 认证失败；本次基于本地 main，未宣称远端最新。
- 基线 `go test ./...`：通过。未执行远端 MySQL 或镜像验收，不等于完整交付门禁通过。
- `go test . -run '^TestDocumentation' -count=1`：通过，覆盖文档布局、链接、元数据和源码归属。
- `git diff --check`：通过。
- 设计自审：核对共享库与迁移职责、全股票刷新与期货独立调度、超时不自动重试、电脑关闭与任务恢复的边界；未发现 TODO/TBD 占位。

## 覆盖率与必要外部门禁

当前未测覆盖率。实现后总覆盖率至少 80%，核心领域各至少 90%。

MySQL 门禁适用：数据库启动初始化归属变化，需要真实隔离库验证。镜像门禁适用：部署角色与镜像目标变化，需分别验证两种镜像。当前两项均 pending；未访问 NAS 业务库，也未部署服务。

## 剩余工作与证据版本

需求与设计需按内容版本获得批准后开始生产实现。实现、完整验证、独立代码评审和部署演练均未完成，禁止将本记录标为 passed 或将变更归档合并。
