# 项目开发宪法

## 1. 项目定位

本项目是 Go 编写的 A 股与大宗商品期货行情、财报、指标、策略扫描与回测平台。当前运行时以版本化行情、持久化任务、不可变扫描快照和版本化 Go 策略为基础，长期方向见 [Roadmap](docs/roadmap.md)。

本文件只保存对全仓任务长期生效的开发规则和文档导航。系统如何工作以 [系统设计](docs/architecture/system-design.md) 和各模块 `DESIGN.md` 为准；本地启动、Docker 与维护流程见 [运行手册](docs/operations.md)。根目录不创建或保留 `README.md`。

## 2. 核心原则

### 2.1 设计文档高于代码

设计文档定义系统应当如何工作，测试证明代码符合设计，代码是设计的可执行实现。

```text
需求意图 → 目标设计 → 测试证据 → 代码实现
```

- 先评审文档，再评审测试和代码。
- 代码与设计文档不一致时，必须立即暂停受影响的实现，向用户说明具体差异、两种处理方式及影响，并询问用户决策。
- 未经用户明确裁决，不得自行认定代码错误、文档过期，或修改其中任意一方来消除冲突。
- 用户决定以设计为准时修正代码和测试；用户决定改变设计时，先显式修订并评审设计，再继续实现。
- 需求文档与模块设计冲突时同样暂停，由用户裁决后在同一变更中更新所有权威文档。

### 2.2 先规划后执行

复杂操作先梳理业务流程、影响模块、失败语义和验证方案。大型需求必须先有批准的需求文档和目标设计；未经批准不编写生产实现。

### 2.3 测试驱动

新增业务逻辑和缺陷修复先写能够失败的测试，再写满足设计的最小实现。总覆盖率保持 80% 以上，核心领域覆盖率保持 90% 以上。

### 2.4 简单、清晰、可维护

- 严格遵守 Karpathy 编码原则：先理解现有流程，做最小正确修改，显式处理假设和边界。
- 遵守 DDD 依赖方向，优先复用现有领域对象、port、版本化仓储和任务模型。
- 不为假设中的扩展点提前增加接口、缓存、消息系统、跨进程协调或兼容层。
- 文件和类型保持单一职责，清理本次变更产生的无用代码。

## 3. 文档体系

### 3.1 权威范围

| 文档 | 权威内容 | 不应包含 |
|---|---|---|
| `AGENTS.md` | 开发流程、全局工程规范、文档导航 | 模块实现细节、完整 API 示例、单次需求设计 |
| `docs/roadmap.md` | 产品与技术方向、阶段目标、进入条件 | 已批准需求正文、个人任务列表 |
| `docs/architecture/` | 跨模块边界、系统流程、全局业务不变量 | 单模块算法和字段清单 |
| 模块 `DESIGN.md` | 模块当前职责、接口、不变量、流程和失败语义 | 提交日志、逐行代码说明 |
| `docs/requirements/` | 大型变更的动机、方案、影响与验收历史 | 长期替代模块设计的孤立规则 |
| `api/api.md` | HTTP 路径、字段、状态码和分页契约 | 内部实现设计 |
| `docs/operations.md` | 启动、配置、Docker、维护与迁移操作 | 产品需求和模块规则 |

同一规则只在最合适的文档完整定义，其他位置使用相对链接。需求完成后，长期有效规则必须合并回模块设计；读者不应依赖历史需求才能理解当前系统。

### 3.2 阅读顺序

开始修改前：

1. 阅读本文件。
2. 从 [领域地图](docs/architecture/domain-map.md) 定位受影响模块。
3. 阅读对应模块 `DESIGN.md` 及其上游架构文档。
4. 修改公开 HTTP 行为时同时阅读 [API 设计](api/DESIGN.md) 与 [HTTP 契约](api/api.md)。
5. 大型需求还必须阅读对应 `docs/requirements/active/REQ-*.md`。
6. 如果阅读源码时发现代码与设计不一致，停止相关工作并询问用户，不得静默“校正文档”或“修正代码”。

### 3.3 模板

- 模块设计使用 [模块设计模板](docs/templates/module-design.md)。
- 大型需求使用 [需求模板](docs/templates/requirement.md)。
- 不建立独立 `docs/decisions/`；方案比较和选择理由保存在需求文档。

## 4. 变更分类与 SOP

### 4.1 大型需求

满足任一条件即为大型需求：新增业务能力或模块；改变公开 API、持久化、策略或引擎语义；调整两个及以上模块边界；包含数据迁移或兼容方案；引入基础设施、外部依赖或并发模型；风险无法由一个局部回归测试充分表达。

流程：

1. 在 `docs/requirements/active/` 创建状态为“待评审”的 `REQ-YYYY-NNN-<topic>.md`。
2. 完成问题、目标、非目标、影响矩阵、方案比较和验收标准，批准后改为“已批准”。
3. 在开发分支先修改所有受影响的架构、模块设计和 API 契约，使其描述合并后的目标状态。
4. 完成设计评审，需求状态改为“开发中”。
5. 按测试驱动实现；新发现的边界先写回需求和设计，再修改实现。
6. 验证通过后改为“待合并”，按需求、设计、测试、代码顺序评审。
7. 记录最终验收，状态改为“已完成”并移入 `docs/requirements/archived/`。

状态固定为：

```text
待评审 → 已批准 → 开发中 → 待合并 → 已完成
              ↘ 已取消
```

### 4.2 轻量缺陷

只有不新增能力、不改变公开契约或已批准语义、局限于单模块、不需迁移且可由明确回归测试证明时，才属于轻量缺陷。

流程：

1. 找到对应模块 `DESIGN.md`。
2. 先补充缺陷暴露的不变量、边界、失败语义或验证证据。
3. 编写失败回归测试。
4. 修复代码并验证“设计—测试—实现”一致。

无需独立需求文档。设计修改必须是长期有效的信息，不追加按日期排列的缺陷流水账。发现跨模块或语义影响时立即升级为大型需求。

### 4.3 机械性维护

纯格式化、拼写修正、可证明等价的重构或不改变行为的依赖补丁，可以不修改设计正文，但变更说明必须声明“无设计语义变化”。无法证明等价时升级为轻量缺陷或大型需求。

### 4.4 评审门禁

评审顺序固定为：

1. 需求评审：目标和验收标准是否完整实现。
2. 设计评审：最终文档是否准确、完整且没有冲突。
3. 测试评审：关键规则是否有可执行证据，覆盖率是否达标。
4. 代码评审：实现是否正确、简单并符合设计。

上游评审未通过，不进入下一层。复杂需求完成后必须启动独立子 Agent 执行代码评审；其他场景尽量不使用子 Agent，确有必要先征得用户同意。

## 5. 架构规范

- 新内核和新增代码的依赖方向固定为 `api/infrastructure/pkg -> application -> port/domain`；现行财报、宏观、旧 HTTP 与迁移链路的兼容例外以 [领域地图](docs/architecture/domain-map.md) 为准，不得继续扩大。
- `internal/market`、`internal/indicator`、`internal/strategy`、`internal/backtest` 不依赖 Gin、GORM、MySQL、HTTP 客户端或具体数据源。
- `api` 只负责传输边界和响应映射；`main.go` 只负责依赖装配与生命周期。
- `internal/application` 编排用例；`internal/port` 只定义真正需要隔离的边界；MySQL 与 Broker 是适配器。
- 外部数据先在适配器完成解析，再进入领域校验；领域不得接收 GORM Model 或外部 JSON DTO。
- `business`、`data`、`model` 只保留财报、宏观和旧迁移兼容职责，不恢复平行技术策略栈。

跨模块设计和完整业务不变量见 [系统设计](docs/architecture/system-design.md)。

## 6. 代码与错误规范

- Go 代码遵守标准编码规范，错误使用 `%w` 保留原因。
- 只在有处理责任的边界记录一次错误，禁止每层重复打印。
- 所有列表、查询、批次和并发都必须有明确上限与稳定排序。
- context 沿调用链传播；取消、超时、失租和普通业务失败必须区分。
- 禁止静默忽略错误；`record not found` 只有契约明确允许时才能转为空。
- 高频循环不逐 Bar、逐 SQL 或逐行输出 Info 日志。

## 7. 日志与安全

- 新内核使用注入的 `log/slog` 或 Telemetry port；纯领域计算不直接记录日志。
- 字段名使用稳定 snake_case，按需记录 component、operation、run_id、instrument、data_version、duration_ms、attempt、error_code。
- Debug 用于诊断，Info 用于任务开始/完成和重要状态，Warn 用于可恢复失败，Error 用于本次操作终止或需人工处理。
- 禁止记录 Token、密码、Cookie、完整配置、请求/响应正文、DSN、路径、堆栈或租约凭据。
- Docker 镜像不得包含本地配置、凭据、数据库转储或导出包，运行时只读挂载配置。

## 8. 数据库与迁移规范

- 数据访问只存在于 `data` 和 `internal/infrastructure/mysql`。
- 简单 CRUD 优先 GORM；复杂批量、锁和性能路径可用参数化原生 SQL，显式列名，禁止新增 `SELECT *`。
- 表名使用 `t_` 前缀，列与索引使用 snake_case；主键为无符号自增整数，业务唯一性由数据库唯一索引保证。
- 时间戳存 UTC `DATETIME(6)`；交易日期使用 date-only；Price/Money 按 10000 缩放整数落库。
- 精确身份使用已验证的二进制列/排序方案，并配套 MySQL 8.4 真实集成测试。
- 事务只包围必须原子提交的数据库操作；禁止在事务内调用外部 HTTP、执行长计算或无界循环。
- 并发写入使用唯一约束、条件更新或明确行锁，锁定顺序稳定。
- 数据库语义变化默认停机更新：停止服务、备份、迁移、校验、启动。操作细节见 [运行手册](docs/operations.md)。

## 9. 测试与交付

- 新增业务逻辑有单元测试，总覆盖率不低于 80%。
- `internal/market`、`internal/indicator`、`internal/strategy/...`、`internal/backtest` 各自覆盖率不低于 90%。
- MySQL 语义必须在获批的远端 8.4/x86_64 服务上，以随机隔离数据库验证；本地不运行 MySQL 验收，部署应用镜像按 `linux/amd64` 构建。测试不得使用现有业务库，SQL mock 或只编译不能替代。
- 快速检查：`npm --prefix web run check && go test ./... && go vet ./...`。
- 完整交付：`bash scripts/verify.sh`。
- 缺少或无法读取本地 `config.yaml` 的数据库配置时，准确报告未完成的 MySQL 门禁；Docker 不可用时准确报告未完成的 amd64 镜像门禁，均不能伪报通过。`config.yaml` 只在本地保存且不得纳入 Git。

## 10. Git 规范

- 默认从 `main` 创建 `codex/` 前缀开发分支；开始前检查工作区，有远程时先同步，不覆盖用户未提交修改。
- 不在 `main` 直接开发。一个 commit 只负责一件事，多项修改拆分提交，commit message 使用英文。
- 完成并验证后合回 `main`，切回 `main` 并清理已合并分支；用户另有明确要求时遵从用户要求。

## 11. 文档索引

### 全局

- [Roadmap](docs/roadmap.md)
- [运行手册](docs/operations.md)
- [系统设计](docs/architecture/system-design.md)
- [领域地图](docs/architecture/domain-map.md)
- [需求模板](docs/templates/requirement.md)
- [模块设计模板](docs/templates/module-design.md)

### 模块

- [API 设计](api/DESIGN.md) / [HTTP 契约](api/api.md)
- [财报与宏观业务](business/DESIGN.md)
- [财报与兼容数据访问](data/DESIGN.md)
- [行情领域](internal/market/DESIGN.md)
- [指标](internal/indicator/DESIGN.md)
- [策略](internal/strategy/DESIGN.md)
- [回测](internal/backtest/DESIGN.md)
- [应用层](internal/application/DESIGN.md)
- [财报筛选](internal/financialscreen/DESIGN.md)
- [应用端口](internal/port/DESIGN.md)
- [MySQL 适配器](internal/infrastructure/mysql/DESIGN.md)
- [外部数据源](pkg/broker/DESIGN.md)
- [行情工作台](web/DESIGN.md)
- [旧行情迁移命令](cmd/migrate-strategy-kernel/DESIGN.md)
