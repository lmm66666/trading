---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["scripts/", "documentation_test.go", "Dockerfile", ".dockerignore"]
related: []
---

# 工程标准

## 1. 代码与错误规范

- Go 代码遵守标准编码规范，错误使用 `%w` 保留原因。
- 只在有处理责任的边界记录一次错误，禁止每层重复打印。
- 所有列表、查询、批次和并发都必须有明确上限与稳定排序。
- context 沿调用链传播；取消、超时、失租和普通业务失败必须区分。
- 禁止静默忽略错误；`record not found` 只有契约明确允许时才能转为空。
- 高频循环不逐 Bar、逐 SQL 或逐行输出 Info 日志。

## 2. 日志与安全

- 新内核使用注入的 `log/slog` 或 Telemetry port；纯领域计算不直接记录日志。
- 字段名使用稳定 snake_case，按需记录 component、operation、run_id、instrument、data_version、duration_ms、attempt、error_code。
- Debug 用于诊断，Info 用于任务开始/完成和重要状态，Warn 用于可恢复失败，Error 用于本次操作终止或需人工处理。
- 禁止记录 Token、密码、Cookie、完整配置、请求/响应正文、DSN、路径、堆栈或租约凭据。
- Docker 镜像不得包含本地配置、凭据、数据库转储或导出包，运行时只读挂载配置。

## 3. 数据库与迁移规范

- 数据访问只存在于 `data` 和 `internal/infrastructure/mysql`。
- 简单 CRUD 优先 GORM；复杂批量、锁和性能路径可用参数化原生 SQL，显式列名，禁止新增 `SELECT *`。
- 表名使用 `t_` 前缀，列与索引使用 snake_case；主键为无符号自增整数，业务唯一性由数据库唯一索引保证。
- 时间戳存 UTC `DATETIME(6)`；交易日期使用 date-only；Price/Money 按 10000 缩放整数落库。
- 精确身份使用已验证的二进制列/排序方案，并配套 MySQL 8.4 真实集成测试。
- 事务只包围必须原子提交的数据库操作；禁止在事务内调用外部 HTTP、执行长计算或无界循环。
- 并发写入使用唯一约束、条件更新或明确行锁，锁定顺序稳定。
- 数据库语义变化默认停机更新：停止服务、备份、迁移、校验、启动。操作细节见 [运行手册](../operations.md)。

## 4. 测试与交付

- 新增业务逻辑有单元测试，总覆盖率不低于 80%。
- `internal/market`、`internal/indicator`、`internal/strategy/...`、`internal/backtest` 各自覆盖率不低于 90%。
- MySQL 语义必须在获批的远端 8.4/x86_64 服务上，以随机隔离数据库验证；本地不运行 MySQL 验收，部署应用镜像按 `linux/amd64` 构建。测试不得使用现有业务库，SQL mock 或只编译不能替代。
- 快速检查：`npm --prefix web run check && go test ./... && go vet ./...`。
- 默认本地门禁：`bash scripts/verify.sh`；包括前端构建/覆盖率、Go 全量/覆盖率、文档、Race、静态、性能和容器配置安全检查。
- SQL、持久化模型/索引、迁移、事务、锁、队列持久化或数据库驱动变化必须追加 `--mysql`；Dockerfile、构建依赖、打包或部署方式变化必须追加 `--image`。同时涉及两类变化用 `--full`（等价于 `--mysql --image`）；CI/发布可显式采用全量。
- 变更评审决定适用门禁，不自动根据文件名猜测。纯文档及本地门禁编排改动没有上述影响时，可记录两项外部门禁为 `not-required` 并写明理由。脚本“未选择”不等于评审认定“不适用”。
- 适用 MySQL 门禁但缺少或无法读取本地 `config.yaml` 时，准确报告未完成；适用镜像门禁但 Docker 不可用时同样报告未完成，均不能伪报通过。`config.yaml` 只在本地保存且不得纳入 Git。


## 5. 文档门禁

`go test . -run '^TestDocumentation' -count=1` 验证必需文档、相对链接和锚点、新变更的生命周期/审批/结果、历史归档例外，以及实质源码唯一 `owns` 归属。元数据检查不能代替人工设计评审或业务测试。

当前规范沿用已有效业务设计；历史批准字段未知时不补造。新复杂变更中 `requirements.md` 唯一保存整体 `status`，`design.md` 使用 `approval_status`，`verification.md` 使用 `result`。检查结果为 `pending/passed/failed/blocked`；只有 `implemented` 且 `result: passed` 可进入 archive。归档历史例外只有 `docs/changes/archive/legacy/`：保留旧 REQ 编号及已完成/已取消状态，不视为新增变更的样板。

适用的必要门禁缺环境或命令失败时记录未通过项目和解除条件，不归档、不合并；不适用项在矩阵中记为 `not-required`，不冒充 passed。验证文件的整体 result 仍只有 pending/passed/failed/blocked，not-required 仅用于单项。前端构建产物、Go 覆盖率报告、本地 config.yaml、数据库凭据均不进入 Git。

## 6. 相关文档

- [项目入口](../../AGENTS.md)
- [系统设计](../architecture/system-design.md)
- [运行手册](../operations.md)
