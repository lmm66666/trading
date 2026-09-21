---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["api/"]
related: []
---

# API 模块设计

想按业务问题阅读，先进入 [项目导航](README.md)；图表与手动刷新的业务流程见 [图表查询](workflows/chart-query.md) 与 [行情采集与版本](workflows/market-data.md)。本文保留传输校验与响应映射契约。

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `api` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

API 是 Gin HTTP 传输适配器，负责路由、请求边界、DTO 校验、调用应用服务、错误映射、响应脱敏和静态前端托管。详细路径、字段、状态码、分页参数和示例以 [HTTP 契约](../standards/http-api.md) 为准。

API 不实现指标、策略、扫描、回测、行情版本或数据库规则；不得导入 GORM、拼写 SQL，或在新内核服务失败时回退到旧技术策略。

## 2. 对外能力与使用者

- `/api/v1` 提供证券搜索、策略目录、图表、版本化行情、手动行情刷新、自选清单、行情看板、回测任务、扫描任务和不可变快照。
- 自选清单 `GET/POST/DELETE /api/v1/watchlist` 由 `WatchlistService` 支撑：变更接口成功后直接返回更新后的完整列表（上限 100 只，超出 409；身份非法 400；未知或非活跃证券 404），不引入二次拉取。
- 行情看板 `GET/POST /api/v1/chart-boards`、`PUT/DELETE /api/v1/chart-boards/:id`、`POST /api/v1/chart-boards/:id/activate` 由 `ChartBoardService` 支撑：五个接口成功后均返回全量状态（boards 按 id 升序 + active_id）；名称/config 校验 400，未知 id 404，超 20 上限 409 BOARDS_FULL，删除最后一块 409 LAST_BOARD；config 以 `json.RawMessage` 透传应用层严格校验。
- Web 工作台与外部客户端共享统一 JSON 包装 `{code,message,data}`。
- 同源静态资源由 `serve_web` 托管，开发模式由 Vite 代理 API。

## 3. 依赖和边界

- handler 通过 `KernelServices` 注入应用服务和只读 port。
- handler 只完成“解析 → Validate → 调用 → 映射”，所有调用传递请求 context。
- 每个独立 HTTP 能力使用单独文件，公共解析和映射放在有限的 handler 辅助代码中。

## 4. 核心不变量

### 4.1 请求边界

- 创建请求使用显式 DTO 和 Validate，JSON body 上限 1MiB，拒绝未知字段、尾随第二个 JSON 值和重复查询参数；`POST /api/v1/market/refresh` 允许空请求体，等价于缺省全量刷新。
- 时间按 RFC3339 接收并规范化为 UTC；日期范围、分页 limit、身份字节长度和指标预算在进入应用层前按契约校验。
- 证券优先使用完整 `Exchange:Code` 身份；六位代码只精确匹配活跃证券，零、一或多结果分别映射为稳定语义。

### 4.2 响应和错误

- Run 输出使用安全 DTO，不返回 RequestJSON、幂等键、租约 owner/token 或底层错误。
- 已知领域/端口错误映射为契约定义的稳定 HTTP 状态和 message；未知内部错误不泄露 SQL、路径、凭据或堆栈。
- 创建和取消异步任务返回 202；成功查询返回 200。HTTP 请求结束不自动取消已接受的持久化任务。

### 4.3 版本与分页

- 图表和行情首次请求允许版本 0，响应必须返回解析后的正 `data_version`；后续页使用相同版本。
- 明细分页使用排他 `after_sequence`，limit 为 1–1000、默认 100。
- 快照首响应返回 SnapshotID；`after_sequence > 0` 必须绑定该 ID 和完整 SnapshotKey，不能重新选择最新快照。
- 服务端 `next_sequence` 是唯一续页依据；满页不保证仍有下一页。

## 5. 主要流程

```text
HTTP 请求
  → 按契约解析 DTO
  → DTO 与身份校验
  → 调用 application 或只读 port
  → 领域错误映射与输出脱敏
  → 统一 JSON 响应
```

图表查询在应用层完整历史上下文计算指标后返回裁剪页；API 只保持请求顺序、版本和游标。workbench 手动刷新经 `UpdaterClient` 发送校验后的 DTO；updater 在独立路由中校验 Token，带证券身份时同步调用 `MarketIngestion.Refresh`，缺省时通过 `MarketTrigger` 异步触发股票全市场补全扫描。updater 不开放工作台 API 或静态页面。

## 6. 失败、取消和一致性语义

- context 取消向下传播；handler 不吞掉取消并继续昂贵计算。
- JSON、字段和身份错误为 400；不存在为 404；幂等/快照/状态冲突为 409；已运行的行情刷新为 429；未分类错误为脱敏 500。
- 取消接口表达“取消请求已持久化”，客户端应重新读取 Run 终态。
- 分页任一页的身份不一致直接失败，不通过切换到最新数据“自愈”。

## 7. 性能与安全约束

- 对 body、页大小、证券范围、日期范围、指标数量和计算成本设置硬上限。
- Handler 日志不得记录完整请求/响应、幂等键、Cookie、Token 或内部错误正文。
- 批量扫描和行情读取必须调用应用层批量能力，API 不按证券循环查询 Repository。

## 8. 测试与验收证据

测试覆盖严格 JSON（含嵌套 config 未知字段与尾随 JSON）、请求限制、错误脱敏、context 传播、身份解析、幂等冲突、固定版本图表、快照分页、自选三接口的校验/幂等/上限/响应结构、看板五接口的校验/上限/末板/NotFound/全量状态信封、手动刷新的两种语义与前端托管。

```bash
go test ./api -cover
```

接口行为变更还必须同步更新 [HTTP 契约](../standards/http-api.md)。

## 9. 相关文档

- [HTTP 契约](../standards/http-api.md)
- [系统设计](../architecture/system-design.md)
- [应用层设计](internal/application.md)
- [端口设计](internal/port.md)


## 图表 STD 契约

图表请求新增 STD(period) 类型，由应用层统一验证/计算，响应单条value序列；HTTP字段形状不变，详细口径见[HTTP契约](../standards/http-api.md)。关联期货仍使用已有market/bars，不新增路由或数据库结构。

## 双服务刷新传输

`updater_router.go` 只注册 `POST /internal/v1/market/refresh` 并校验 Bearer Token（至少 32 字节可打印非空白 ASCII，哈希后常量时间比较）。`updater_client.go` 只调用该固定路径，服务地址来自配置而非请求；限制请求/响应 1MiB、40 秒总超时、专用 Transport 仅用 HTTP/1 且去除 GetBody 重放能力、不重试 POST、不跟随重定向。成功响应以类型化 DTO 重建，错误仅允许既定状态/message，不透传上游错误正文。

网络故障为 503 UPDATER_UNAVAILABLE，超时为 504 UPDATER_TIMEOUT，认证或响应异常为 502 UPDATER_BAD_RESPONSE。内部 401 不作为浏览器认证问题返回。单证券取消随请求传播，全市场受理后的生命周期属于 updater 根 context，断线不保证未执行。详细协议以 HTTP 契约为准。
