# 领域与模块地图

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 最后更新 | 2026-09-14 |
| 权威范围 | 模块职责、依赖边界与设计文档导航 |

## 模块地图

| 模块 | 核心职责 | 允许依赖 | 设计或契约 |
|---|---|---|---|
| 组合根（`main.go`、`config`） | 配置、依赖装配、进程生命周期 | 所有需要装配的具体实现 | [系统设计](system-design.md) |
| `api` | HTTP 校验、应用调用、响应映射、兼容适配 | business、application、backtest、market、port、strategy、Gin | [API 设计](../../api/DESIGN.md)、[HTTP 契约](../../api/api.md) |
| `business` | 财报、宏观查询、信号与旧调度业务 | data、model、broker、`pkg/indicator`、financialscreen | [业务设计](../../business/DESIGN.md) |
| `data` / `model` | 财报和证券主数据访问；旧 K 线迁移/回滚模型 | config、MySQL adapter、GORM、model | [旧数据访问设计](../../data/DESIGN.md) |
| `internal/market` | 行情值对象、Bar、Dataset、复权和周线 | Go 标准库 | [行情领域设计](../../internal/market/DESIGN.md) |
| `internal/indicator` | 带有效位的指标序列与计算图 | market、Go 标准库 | [指标设计](../../internal/indicator/DESIGN.md) |
| `internal/strategy` | 策略定义、注册、实例和时间线回放 | market、indicator | [策略设计](../../internal/strategy/DESIGN.md) |
| `internal/backtest` | 账户、撮合、费用、公司行动、结果和指标 | market、indicator、strategy | [回测设计](../../internal/backtest/DESIGN.md) |
| `internal/application` | 行情、图表、扫描、回测和 Worker 用例编排 | port、market、indicator、strategy、backtest | [应用层设计](../../internal/application/DESIGN.md) |
| `internal/financialscreen` | 财报筛选规则与组合 | model 中的财报值 | [财报筛选设计](../../internal/financialscreen/DESIGN.md) |
| `internal/port` | 应用所需的存储、数据源、任务与遥测边界 | market、backtest、标准库 | [端口设计](../../internal/port/DESIGN.md) |
| `internal/infrastructure/mysql` | 版本化行情、任务、租约、结果和迁移持久化 | port、market、backtest、兼容 model、GORM/MySQL | [MySQL 设计](../../internal/infrastructure/mysql/DESIGN.md) |
| `pkg/broker` | 新浪与东方财富外部数据源适配 | port、market、兼容 model、HTTP | [Broker 设计](../../pkg/broker/DESIGN.md) |
| `pkg/indicator` | 旧财报指标辅助和并发限流器 | 兼容 model、标准库 | 归入 [Broker 设计](../../pkg/broker/DESIGN.md) 的限频边界与 [业务设计](../../business/DESIGN.md) 的旧指标边界 |
| `web` | React 行情工作台和 API 客户端状态 | React、Lightweight Charts、HTTP API | [前端设计](../../web/DESIGN.md) |
| `cmd/migrate-strategy-kernel` | 旧行情到版本化内核的一次性迁移 | config、broker、MySQL adapter、GORM | [迁移设计](../../cmd/migrate-strategy-kernel/DESIGN.md) |
| `scripts` / `shell` | 验证门禁和旧批量运维入口 | CLI 工具 | 对应调用模块的设计与 [系统设计](system-design.md) |

## 边界规则

1. 业务规则向领域模块收敛，不在 API、GORM Model 或外部 DTO 中重复实现。
2. 应用层可以编排多个端口和领域模块，但不导入 Gin、GORM 或具体 Broker。
3. 基础设施和 Broker 可以依赖端口与领域对象，领域不得反向依赖适配器。
4. `business`/`data`/`model` 只维护财报、宏观和迁移兼容能力，不恢复已经删除的旧技术策略实现。
5. 只有具备长期独立职责的目录维护 `DESIGN.md`；配置、兼容 Model 和小型工具归入最近的权威模块，避免形式化拆文档。
6. 跨模块约束由 [系统设计](system-design.md) 定义；模块文档只补充该模块如何满足约束。

### 当前兼容依赖

上表记录当前代码实际允许的直接依赖，不把遗留边误写成已经完成的目标架构。以下依赖只服务现有兼容职责，新代码不得继续扩大：

- `api -> business -> data/model/pkg` 保留财报、宏观和旧 HTTP 行为。
- `data -> config/MySQL adapter` 是现有数据库初始化与兼容数据访问桥接。
- MySQL adapter 对 `backtest` 和 `model` 的依赖用于结果映射与旧行情迁移。
- `pkg/broker`、`pkg/indicator` 对 `model` 的依赖用于旧财报、宏观和实时行情接口。

消除这些依赖会改变模块边界或兼容行为，必须先建立大型需求；候选方向见 [Roadmap](../roadmap.md)。

## 阅读规则

- 修改模块前，先阅读该模块 `DESIGN.md` 及其链接的上游系统设计。
- 修改公开 HTTP 行为时，同时阅读 `api/DESIGN.md` 与 `api/api.md`。
- 修改跨模块流程时，从系统设计定位数据流，再读取所有受影响模块设计。
- 发现模块职责、代码依赖与本地图不符时，立即暂停受影响工作，列明差异、两种处理方式及影响并请求用户裁决；未经裁决不得修改代码或设计来消除冲突。
