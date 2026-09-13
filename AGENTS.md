# 项目概述

本项目是 Go 编写的 A 股行情、财报、指标、策略扫描与回测平台。技术策略直接用 Go 实现，运行时使用版本化行情、持久化任务和不可变扫描快照；长期方向是逐步提供类似 TradingView 的指标、脚本和回测能力。

## 根目录文档规则

- 项目根目录只使用 `AGENTS.md` 承载项目概述、架构、启动、迁移、部署和开发约束，不创建或保留根目录 `README.md`。
- 修改根目录说明时直接更新 `AGENTS.md`，禁止把内容拆回根目录 `README.md`。
- 子目录可保留用于模块约束和实现说明的 `README.md`，不受本规则影响。

## 当前架构

```text
trading/
├── main.go                         # 组合根：HTTP、Worker、行情调度与优雅停机
├── config/                         # DB 与 Worker 配置
├── api/                            # Gin API、V1 持久化任务和兼容适配器
├── business/                       # 财报与宏观业务；不再承载技术策略
├── data/                           # 财报/旧表迁移回滚 Repository
├── model/                          # 财报及迁移兼容模型
├── internal/
│   ├── market/                     # 行情领域值、Bar、Dataset、复权与对齐
│   ├── indicator/                  # 带有效位的指标图
│   ├── strategy/                   # 策略接口、时间线、Registry 与内置 Go 策略
│   ├── backtest/                   # 账户、撮合、成本、回测与指标
│   ├── application/                # 行情采集/查询/调度、回测/扫描、持久化 Worker
│   ├── financialscreen/            # 财报筛选器
│   ├── port/                       # 应用端口与 DTO
│   └── infrastructure/mysql/       # 版本化行情、任务、结果和快照适配器
├── pkg/broker/                     # 新浪财报/宏观与东方财富版本化行情适配器
├── web/                            # React 行情工作台、K 线与指标交互
├── cmd/migrate-strategy-kernel/    # 旧行情 dry-run、迁移、检查点与重跑命令
├── scripts/verify.sh               # 覆盖率、Race、集成、性能和镜像门禁
└── shell/                          # 旧批量运维脚本
```

旧 `pkg/filter`、`pkg/strategy`、`business.StockDataService` 和旧行情调度器已删除。旧 K 线 Repository/Model 只供迁移或回滚工具使用，生产服务不读取、不写入，也不在正常启动时执行其 `AutoMigrate`。

## 本地启动

要求 Go 1.25.7+、Node.js 24+、MySQL 5.7 或 8.0。创建 `trading` 数据库后：

```bash
cp config.example.yaml config.yaml
# 编辑本地 config.yaml；该文件不会进入镜像
go mod download
npm --prefix web ci
npm --prefix web run build
go run . -config config.yaml
```

服务默认监听 `:8080`，同源提供行情工作台与 API。前端开发可运行 `npm --prefix web run dev`，由 Vite 把 `/api` 代理到 `:8080`。启动时只迁移财报、证券主数据和新策略内核表，不再创建或写入旧技术 K 线表。`Worker` 配置分别控制持久化任务 Worker 数、租约、轮询、兼容接口同步等待时间，以及扫描/行情刷新的有界并发数。

## 运行约束

- Go 1.25.7+；MySQL 5.7 或 8.0。
- 时间戳统一使用 UTC `time.Time` / `DATETIME(6)`；交易日期使用独立 date-only 值。
- 新计算只读取大于 0 的 `COMPLETE` 行情版本，任务执行期间不得重新解析最新版本。
- Price/Money 使用缩放 10000 的有符号整数；版本和序号使用无符号整数。
- 同一策略行为变更必须新增策略版本；撮合、成本或引擎语义变化必须提升 `EngineVersion`。
- 信号在当前 Bar 收盘后产生，订单最早在下一根可交易 Bar 开盘成交，禁止未来数据。
- 全市场扫描必须通过 `BatchDatasets` 有界批量加载，不得恢复逐证券 Repository 查询。
- 不透明身份字段按 UTF-8 字节限制并精确区分大小写和尾空格。
- Docker 镜像不得包含本地配置、凭据、数据库转储或导出包；运行时只读挂载配置。

## 策略与任务模型

- 内置策略版本 1：`daily_b1_buy`、`weekly_b1_buy`、`bottom_surge_pullback`。`EngineVersion` 为 `strategy-kernel-v1`；策略行为变化必须新增策略版本或提升引擎版本。
- 信号严格按时间线重放。第 N 根 Bar 产生的订单最早在第 N+1 根可交易 Bar 开盘成交，指标预热期返回无效值；策略不可读取未来数据。
- 旧实现可能使用未来数据；修复后，同一历史区间的信号和收益可能与旧结果不同，这是预期行为变化。
- 回测/扫描任务持久化在 MySQL，HTTP 请求结束或同步等待超时不会取消任务。任务固定行情版本、证券集合、策略版本、参数、执行配置和引擎版本，可重复复现。
- 扫描一次批量读取最多 5000 只证券并发布不可变快照；单只证券失败可得到 `PARTIAL_SUCCEEDED`，不会丢弃其他成功结果。

`GET /api/v1/instruments` 搜索证券，`POST /api/v1/chart-queries` 按固定行情版本返回 K 线、成交量与 SMA/EMA/MACD/KDJ 指标。`POST /api/v1/backtest-runs` 与 `POST /api/v1/scan-runs` 创建任务；状态、取消、成交、权益和最新扫描快照详见 `api/api.md`。旧 `/api/stocks/*` 路径保留为兼容适配器，但行情保存、增量刷新、价格查询、信号和回测均已切换到新内核。

## Docker

镜像不包含任何本地配置，进程以非 root 用户运行。必须只读挂载配置：

```bash
docker buildx build --platform linux/amd64 -t trading:latest --load .
docker run -d --name trading -p 8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  trading:latest
```

也可以覆盖容器命令指定其他容器内路径：`trading:latest -config /run/secrets/trading.yaml`。

## Redis / Kafka 取舍

第一阶段保持 MySQL-only：任务、租约、幂等、快照和行情版本处于同一事务边界，5000 证券扫描已使用批量读取和固定 Worker。仅在出现以下指标后引入额外组件：

- MySQL 队列领取或续租 P95 持续超过 100ms，或待执行任务积压超过一个扫描周期：优先迁移到 Redis Streams；MySQL 继续保存最终结果和审计记录。
- 行情需要多个独立消费者、跨服务重放，或峰值吞吐达到单库写入瓶颈：再使用 Kafka 作为行情事件日志；数据库发布仍需保持数据版本原子性。
- 不因“逐股查询慢”单独引入 Redis/Kafka；该瓶颈已通过一次批量加载全市场数据解决，缓存或消息系统不能替代正确的查询模型和索引。

## 开发规范

### 架构

- 依赖方向固定为 `api/infrastructure/pkg -> application -> port/domain`。`internal/market`、`internal/indicator`、`internal/strategy`、`internal/backtest` 不得依赖 Gin、GORM、MySQL、HTTP 客户端或具体数据源。
- `api` 只负责参数解析、鉴权边界、调用 application service 和响应映射，不承载业务规则；`main.go` 只负责依赖装配和生命周期管理。
- `internal/application` 编排用例、事务边界外的流程和重试；`internal/port` 只定义领域真正需要替换或隔离的边界；`internal/infrastructure/mysql` 实现持久化；`pkg/broker` 实现外部数据源适配。
- 新功能优先复用现有领域对象、port、版本化行情仓储和任务模型。禁止为同一业务另建平行的 service/repository/model 栈，也不为只有一个简单实现的内部函数预先抽象接口。
- 外部数据必须先在 adapter 边界完成结构解析，再进入领域校验；领域层不得接收 GORM Model、外部 JSON DTO 或未校验的字符串枚举。
- 文件和类型保持单一职责。仅在当前需求确实需要时增加抽象、缓存、消息系统、跨进程协调或兼容层，避免为假设中的扩展点提前设计。

### 日志

- 新内核代码统一使用注入的 `log/slog` 或现有 Telemetry port；领域纯计算包不直接记录日志。旧 `business` 中的标准库 `log` 可按需逐步迁移，不因无关需求整体重写。
- 日志采用结构化字段，字段名使用稳定的 `snake_case`。按场景记录 `component`、`operation`、`run_id`、`instrument`、`data_version`、`duration_ms`、`attempt`、`error_code` 等可检索信息。
- `Debug` 用于本地诊断，`Info` 用于任务开始/完成和重要状态变化，`Warn` 用于可恢复失败或部分降级，`Error` 用于本次操作终止或需要人工处理的问题。
- 错误沿调用链使用 `%w` 保留原因，只在有处理责任的边界记录一次；禁止每层重复打印同一个错误。批量任务汇总成功/失败数量，逐项失败只记录必要身份和稳定错误码。
- 禁止记录 Token、密码、Cookie、完整配置、完整请求/响应正文、数据库连接串或其他敏感信息。外部坏响应只记录状态码、内容类型、长度及必要的摘要。
- 高频循环不逐 Bar、逐 SQL 或逐行输出 Info 日志；性能和阶段耗时通过聚合字段或 Telemetry 记录。

### 数据库与 SQL

- 数据访问只存在于 `data` 和 `internal/infrastructure/mysql`。业务、领域和 API 层不得直接调用 GORM 或拼写 SQL。
- 简单 CRUD 优先使用 GORM；复杂批量查询、锁和性能敏感路径可以使用原生 SQL。原生 SQL 必须参数化，禁止通过字符串拼接写入值，并显式列出所需列，禁止新增 `SELECT *`。
- 表名使用 `t_` 前缀，列名和索引名使用 `snake_case`；主键使用无符号自增整数，业务唯一性同时由数据库唯一索引保证，不能只依赖应用层检查。
- 时间戳统一存 UTC `DATETIME(6)`；交易日期使用独立 date-only 列；Price/Money 按缩放 10000 的整数落库，禁止为方便改回浮点。
- 身份键必须精确保留大小写和尾空格语义时，使用已验证的 binary collation/二进制列方案，并配套 MySQL 5.7、8.0 集成测试。
- 查询必须有明确边界和稳定排序。批量读取/写入需要限制批次大小，避免超过 MySQL 参数或 packet 限制；新增查询需检查其过滤、排序和分页是否有匹配索引。
- 事务只包围必须原子提交的数据库操作；禁止在事务内调用外部 HTTP、执行长时间计算或无界循环。并发写入依赖数据库唯一约束、条件更新或明确行锁，锁定顺序必须稳定。
- 所有数据库错误必须返回并带操作上下文，禁止静默忽略。`record not found` 只有在业务明确允许时才能转换为空结果。

### 数据库变更与停机更新

- 本项目允许停机更新。涉及表结构或数据语义变化时，默认流程为：停止服务、备份数据库、执行迁移、运行校验、启动新版本。
- 不默认建设双写、影子表、灰度读、在线回填、跨版本长期兼容或复杂回滚状态机。只有用户明确要求不停机或实际数据量证明必须在线迁移时才增加这些机制。
- 简单新增表/列可由受控 `AutoMigrate` 完成；删除、重命名、约束/索引调整和数据重写使用显式、可审查的迁移语句或命令。
- 迁移应可安全重跑或在执行前检测当前状态。破坏性变更必须在备份后执行，并提供最小校验查询；回滚默认使用旧程序加数据库备份恢复。

### 测试

- 遵循 Go 标准编码规范，新增业务逻辑配套单元测试。
- 总测试覆盖率保持 80% 以上；`internal/market`、`indicator`、`strategy/...`、`backtest` 各自保持 90% 以上。
- 快速检查：`npm --prefix web run check && go test ./... && go vet ./...`。
- 完整交付检查：`bash scripts/verify.sh`。
- MySQL 集成测试需要 Docker；环境不可用时必须明确报告，不能用 SQL mock 或仅编译代替真实 MySQL 5.7/8.0 验证。

## 迁移与接口

- 新计算只读取大于 0 的 `COMPLETE` 数据版本。先执行只读检查：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true -batch-size=1000
```

- 确认报告无缺失后，在维护窗口写入新表：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size=1000
```

- 当前策略内核迁移命令支持检查点和幂等重跑，不会推断证券的 `active` 与 `lot_size`。上线前需补齐并激活 `t_instruments`；六位代码兼容接口只精确匹配活跃证券。
- 后续数据库变更默认按“停服务、备份、迁移、校验、启动”执行，不沿用该历史迁移任务中的复杂切换设计，除非具体需求明确需要。
- `/api/v1` 使用显式策略版本、幂等键、持久化 Run 与 SnapshotID 游标。
- 兼容 `signal` 只读最新已发布快照；兼容 `backtest` 创建持久化任务；兼容历史保存、增量刷新和价格查询均使用版本化行情服务。
- API 请求限制、状态码、固定执行假设和分页语义以 `api/api.md` 为准。

## 完整验证

```bash
bash scripts/verify.sh
```

脚本执行总覆盖率与核心领域覆盖率门禁、全量测试、Race Detector、`go vet`、5000 证券性能测试、MySQL 5.7/8.0 集成测试，以及不包含本地配置的镜像构建。MySQL 集成测试和镜像构建需要可用的 Docker daemon；SQL mock 不能代替真实事务与并发验证。
