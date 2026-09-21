---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["scripts/", "documentation_test.go", "verification_test.go", "Dockerfile", ".dockerignore", "compose.nas.yaml", "Makefile"]
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
- 普通 Docker 镜像不得包含本地配置、凭据、数据库转储或导出包，运行时只读挂载配置。用户于 2026-09-21 明确批准私有 NAS `updater-configured` 目标内置 `config.updater.yaml`：该文件不得提交或进入普通构建上下文，配置副本为 app:app、0400；镜像与 tar 按含凭据文件管理，不公开发布。

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

验收按改动风险选择，不因一次小改动反复执行全项目检查。用户于 2026-09-21 明确要求兼顾效率、简化流程；下列规则替代原“每次全量本地门禁，再追加外部验收”。

| 改动范围 | 必要检查 |
|---|---|
| 纯文档 | 文档契约与 diff 检查 |
| 局部业务逻辑或缺陷 | 相关模块回归测试、相关构建/静态检查；新增逻辑验证覆盖率 |
| 跨模块应用改动 | 默认快速检查 `bash scripts/verify.sh` |
| 并发、锁或任务生命周期 | 追加受影响包的 `go test -race`；无需无关包重复 Race |
| SQL、表结构、迁移、数据库事务/驱动 | 独立运行 `--mysql`，使用获批远端 MySQL 8.4/x86_64 随机隔离库；不能用 SQL mock 或业务库替代 |
| 配置打包或单个镜像 | 独立运行 `--image=updater`、`--image=workbench` 或 `--image=updater-configured`；不自动跑应用全量测试或 MySQL |
| Dockerfile 公共构建阶段 | `--image` 检查全部目标 |
| 验收脚本 | 调度、失败传播、shell 语法及文档测试；无需为了修改调度逻辑再次连接所有外部服务 |
| 大范围重构、全量发布核验 | 显式 `--full`，覆盖全量覆盖率、Race、数据库和全部镜像 |

- 默认快速检查运行前端测试/生产构建、Go 测试和 vet；Go 测试本身已覆盖文档和全市场性能回归，不单独重复执行。
- 覆盖率目标不降低：总计至少 80%，核心 market、indicator、strategy、backtest 各至少 90%。新增业务逻辑检查受影响模块；全量覆盖率在 `--full` 或需要重新核实全局水平时运行，文档/配置/打包无需重跑覆盖率。全量 Go 覆盖率只运行一次，核心指标从同一报告提取。
- `--mysql`、`--image[=目标]` 是独立检查，可组合；`--mysql --image` 不包含本地全量检查，只有 `--full` 执行全部。无参数才运行默认快速检查。
- 前端依赖已安装时复用 node_modules；首次使用自动安装，修改 package.json/package-lock.json 或切换到不同依赖版本后先执行 `npm --prefix web ci --prefer-offline`。CI 使用干净工作区或显式安装依赖，不能使用与锁文件不一致的依赖证明通过。
- Docker 内容缓存正常复用；仅内置配置阶段必须用 `--no-cache-filter updater-configured`，防止 secret 内容变化仍使用旧配置。镜像固定 linux/amd64，检查非 root、默认启动命令和各目标配置边界。
- 同一源码、配置与相关环境已有通过证据时直接复用；只有新改动、失败或未解决风险才扩大/重跑。网络下载失败记录为环境问题，定位后只重试失败步骤，不重复已通过的无关检查；不无限重复全无缓存下载。
- 适用检查失败或缺前提要明确报告；不适用项记 `not-required` 及理由，未执行不算通过。真实配置与凭据不提交，测试不得修改业务库。

## 5. 文档门禁

`go test . -run '^TestDocumentation' -count=1` 验证必需文档、相对链接和锚点、新变更的生命周期/审批/结果、历史归档例外，以及实质源码唯一 `owns` 归属。元数据检查不能代替人工设计评审或业务测试。

当前规范沿用已有效业务设计；历史批准字段未知时不补造。新复杂变更中 `requirements.md` 唯一保存整体 `status`，`design.md` 使用 `approval_status`，`verification.md` 使用 `result`。检查结果为 `pending/passed/failed/blocked`；只有 `implemented` 且 `result: passed` 可进入 archive。归档历史例外只有 `docs/changes/archive/legacy/`：保留旧 REQ 编号及已完成/已取消状态，不视为新增变更的样板。

适用的必要门禁缺环境或命令失败时记录未通过项目和解除条件，不归档、不合并；不适用项在矩阵中记为 `not-required`，不冒充 passed。验证文件的整体 result 仍只有 pending/passed/failed/blocked，not-required 仅用于单项。前端构建产物、Go 覆盖率报告、本地 config.yaml、数据库凭据均不进入 Git。

## 6. 相关文档

- [项目入口](../../AGENTS.md)
- [系统设计](../architecture/system-design.md)
- [运行手册](../operations.md)

## 双服务镜像验收

Dockerfile 提供 updater/workbench 两个目标；updater 镜像不构建或包含前端，workbench 包含静态产物。两者按 linux/amd64 构建，非 root 运行，只读挂载本地配置。`--image` 实际构建全部目标，`--image=目标` 仅构建所选目标。`--mysql` 同时执行 MySQL 模块与 data 的隔离集成测试，以覆盖 workbench 不执行 DDL 的职责。NAS Compose 使用已存在的 MySQL，不声明新的 MySQL 容器或数据卷。

内置配置目标由 `make image-updater-configured` 构建，通过 BuildKit secret 提供输入，并强制该阶段重新执行，避免配置变化命中旧缓存。选择内置配置目标时用示例配置检查文件读取权限与默认命令；真实配置内容仅做一致性校验，不写入日志。
