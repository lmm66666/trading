---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: []
related: []
---

# 功能目录与源码地图

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 最后更新 | 2026-09-14 |
| 权威范围 | 模块职责、依赖边界与设计文档导航 |

## 模块地图

| 模块 | 核心职责 | 允许依赖 | 设计或契约 |
|---|---|---|---|
| 组合根（`main.go`、`config`） | 配置、依赖装配、进程生命周期 | 所有需要装配的具体实现 | [系统设计](../architecture/system-design.md) |
| `api` | HTTP 校验、应用调用、响应映射 | application、backtest、market、port、strategy、Gin | [API 设计](api.md)、[HTTP 契约](../standards/http-api.md) |
| `data` / `model` | 连接初始化；旧库迁移链路依赖的兼容模型 | config、MySQL adapter、GORM、model | [兼容数据访问设计](data.md) |
| `internal/market` | 行情值对象、Bar、Dataset、复权和周线 | Go 标准库 | [行情领域设计](internal/market.md) |
| `internal/indicator` | 带有效位的指标序列与计算图 | market、Go 标准库 | [指标设计](internal/indicator.md) |
| `internal/strategy` | 策略定义、注册、实例和时间线回放 | market、indicator | [策略设计](internal/strategy.md) |
| `internal/backtest` | 账户、撮合、费用、公司行动、结果和指标 | market、indicator、strategy | [回测设计](internal/backtest.md) |
| `internal/application` | 行情、图表、扫描、回测和 Worker 用例编排 | port、market、indicator、strategy、backtest | [应用层设计](internal/application.md) |
| `internal/port` | 应用所需的存储、数据源、任务与遥测边界 | market、backtest、标准库 | [端口设计](internal/port.md) |
| `internal/infrastructure/mysql` | 版本化行情、任务、租约、结果和迁移持久化 | port、market、backtest、兼容 model、GORM/MySQL | [MySQL 设计](internal/infrastructure/mysql.md) |
| `pkg/broker` | 新浪与东方财富外部数据源适配 | port、market、HTTP | [Broker 设计](pkg/broker.md) |
| `web` | React 行情工作台和 API 客户端状态 | React、Lightweight Charts、HTTP API | [前端设计](web.md) |
| `cmd/migrate-strategy-kernel` | 旧行情到版本化内核的一次性迁移 | config、broker、MySQL adapter、GORM | [迁移设计](cmd/migrate-strategy-kernel.md) |
| `scripts` | 验证门禁 | CLI 工具 | [工程标准](../standards/engineering.md) |

## 边界规则

1. 业务规则向领域模块收敛，不在 API、GORM Model 或外部 DTO 中重复实现。
2. 应用层可以编排多个端口和领域模块，但不导入 Gin、GORM 或具体 Broker。
3. 基础设施和 Broker 可以依赖端口与领域对象，领域不得反向依赖适配器。
4. `data`/`model` 只维护旧库迁移链路所需的兼容模型与连接初始化，不恢复已经删除的财报、宏观或旧技术策略实现。
5. 只有具备长期独立职责的目录维护当前设计文档；配置、兼容 Model 和小型工具归入最近的权威模块，避免形式化拆文档。
6. 跨模块约束由 [系统设计](../architecture/system-design.md) 定义；模块文档只补充该模块如何满足约束。

### 当前兼容依赖

上表记录当前代码实际允许的直接依赖，不把遗留边误写成已经完成的目标架构。以下依赖只服务现有迁移兼容职责，新代码不得继续扩大：

- `data -> config/MySQL adapter` 是现有数据库初始化与迁移链路桥接；`data` 包整体收敛进组合根属后续变更。
- MySQL adapter 对 `backtest` 和 `model` 的依赖用于结果映射与旧行情迁移；迁移验收后由后续变更删除迁移链路并 DROP 旧表。

消除这些依赖会改变模块边界或兼容行为，必须先建立大型需求；候选方向见 [Roadmap](../roadmap.md)。

## 阅读规则

- 修改模块前，先阅读该模块设计 及其链接的上游系统设计。
- 修改公开 HTTP 行为时，同时阅读 [API 设计](api.md) 与 [HTTP 契约](../standards/http-api.md)。
- 修改跨模块流程时，从系统设计定位数据流，再读取所有受影响模块设计。
- 发现模块职责、代码依赖与本地图不符时，立即暂停受影响工作，列明差异、两种处理方式及影响并请求用户裁决；未经裁决不得修改代码或设计来消除冲突。

## 源码目录与职责

```text
main.go                         # 组合根、进程与依赖生命周期
config/                         # 本地配置解析，归系统设计
api/                            # HTTP 与静态前端传输适配
data/                           # 连接初始化与迁移链路桥接
model/                          # 旧库迁移兼容模型，归 data
internal/
  market/                       # 行情领域、身份、日期、价格和 Dataset
  indicator/                    # 纯指标计算与计算图
  strategy/                     # 策略契约、Registry、Replay
    builtin/                    # 版本化内置策略，归 strategy
  backtest/                     # 账户、撮合、成本和回测结果
  application/                  # 查询、采集、扫描、回测和 Worker 用例
  port/                         # 应用的存储、来源、任务与遥测边界
  infrastructure/mysql/         # 数据模型、迁移、队列与持久化
    dbtest/                     # 远端随机隔离数据库测试夹具，归 MySQL
pkg/
  broker/                       # 外部来源解析与限频
cmd/migrate-strategy-kernel/     # 离线旧行情迁移入口
web/
  src/api/                      # HTTP 客户端与 DTO
  src/features/search/          # 证券搜索交互
  src/features/chart/           # 行情图表与分页状态
  src/features/indicators/      # 指标参数交互
  src/test/                     # 前端测试公共配置
  src/                          # App、启动与样式
  vite.config.ts                # 构建和测试配置，归 web
scripts/                        # 完整验收，归工程标准
Dockerfile、.dockerignore        # 镜像与构建边界，归工程标准
config.example.yaml             # 非敏感配置样例，归系统设计
*_test.go、*.test.ts(x)          # 测试与其被测模块共享归属
```

源码树为导航而非递归归属声明；精确归属以文档 frontmatter 的 `owns` 为准。目录项必须以 `/` 结尾，只覆盖直接文件；不接受 glob。`related` 表示依赖关系，不转移所有权。每个 Go、TS/TSX、JS/JSX、CSS、SQL 和 shell 源文件（含测试）必须恰有一个所有者；生成产物和依赖目录不纳入。新增目录须显式补充归属。

## 继承基线与验证范围

当前设计正文继承自 `8693e59` 的“当前有效”文档；本次只迁移位置、导航和归属，不把它们重新标成代码提取的草稿。历史批准人、时间、内容版本未结构化保存的字段保持 `null`；本次治理批准不替代历史业务批准。`baseline_revision` 表示迁移来源，不能解释为测试已执行或已覆盖全部业务。

`owns` 和链接检查只验证文档可发现性，不能证明业务实现符合设计。新变更的实际检查与批准证据由 [本次迁移记录](../changes/archive/2026-09-17-document-driven-development/requirements.md) 记录；长期流程入口见 [AGENTS](../../AGENTS.md)。

既有 `docs/superpowers/` 为迁移前实现计划和设计草稿，`docs/analysis/` 为研究材料，均不属于当前规范来源；本次保留历史内容，仅更新必要链接。后续不在项目中新增临时实施计划或通用流程模板。
