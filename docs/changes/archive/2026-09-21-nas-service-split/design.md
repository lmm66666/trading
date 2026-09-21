---
id: CHG-2026-09-21-NAS-SERVICE-SPLIT-DESIGN
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T11:55:59+08:00"
approved_revision: "03abaed"
approved_scope: ["REQ-SPLIT-001–010", "design.md sections 1–8"]
---

# NAS 服务拆分目标设计

需求来源见 [需求](requirements.md)。本文描述合并后的目标；批准前不覆盖现有系统设计。

## 1. 当前与目标行为

当前 `main.go` 的 `run/newKernel` 同时装配 HTTP、静态前端、扫描/回测 Worker、股票和期货调度。`data.New` 每次连接均执行 schema 初始化。手动刷新直接调用本进程的采集器或调度器。

目标采用同仓库、共享领域与应用代码、两个独立部署进程。共用同一个业务 MySQL，按运行角色分配写入职责。

```text
新浪来源 → NAS updater → NAS MySQL ← 电脑 workbench ← 本机浏览器
                 ↑                        │
                 └── HTTP 手动刷新请求 ────┘
```

| 角色 | 运行内容 | 数据写入 |
|---|---|---|
| updater（NAS） | schema 初始化、Broker、股票调度、可选期货调度、受保护的刷新 HTTP 接口 | 证券主数据、行情版本、日/周线、因子等现有行情发布数据 |
| workbench（电脑） | 前端托管、现有业务 API、指标、扫描/回测 Worker、刷新 HTTP 客户端 | 自选、看板、计算任务、扫描快照、回测结果和任务事件 |

NAS updater 不托管前端、不运行扫描/回测 Worker，也不暴露这些业务路由。workbench 不装配任何 Broker、行情调度或本地刷新实现。

CLI 使用必填 `-service updater|workbench`，保留 `-config`。缺失或非法角色在连接数据库前失败；不提供 all 模式。公共配置加载、HTTP 生命周期和连接管理按必要范围提取为共享组合根代码，不复制业务逻辑。

## 2. 数据来源、表关系与迁移归属

两服务正常使用同一个 NAS 业务库，工作台经局域网直接读取行情，继续使用现有 MySQL 适配器及 `BatchDatasets` 批量路径。浏览器只连接本机 API，不接触数据库和服务 Token。

- 行情集合：`t_instruments`、`t_market_data_versions`、`t_market_bars`、`t_adjustment_factors`、`t_corporate_actions`。发布和历史版本有效区间保持现有事务规则，读取只接受 COMPLETE。
- 工作台集合：`t_watchlist`、`t_chart_boards`、`t_compute_runs`、`t_signal_snapshots`、`t_signal_snapshot_rows`、`t_backtest_runs`、`t_backtest_orders`、`t_backtest_trades`、`t_backtest_equity_points`、`t_outbox_events`。证券关联、固定版本、Run/SnapshotID 及序列分页保持现有规则。
- 不增加跨数据库关联；不修改列、外键语义、唯一约束或查询索引。现有版本分配锁、历史查询与批量索引仍由 MySQL 设计负责。
- updater 启动前完成现有 `data.New` schema 初始化，然后开放接口、启动调度。workbench 使用只连接和配置连接池的入口，不执行 AutoMigrate；目标库需先初始化。
- schema 初始化完成不等于行情已准备好。首次更新尚未发布 COMPLETE 时保留现有无数据错误，不伪造版本；已有 NAS 库的 COMPLETE 数据可立即使用。
- 为最小化变更，updater 仍初始化现有完整 schema，不拆出两套迁移或额外迁移服务。后续 schema 升级仍按停机流程执行，禁止边运行边升级两个不同版本的进程。
- 本地 mock 库是独立开发环境。工作台和对应更新服务应配置为同一目标库；不进行启动时自动复制、自动切库或自动修复不匹配配置。运行手册明确核对两份配置的目标库。

## 3. 手动刷新接口

电脑端继续提供 `POST /api/v1/market/refresh`；NAS 仅增加专用 `POST /internal/v1/market/refresh`，同样采用 `{code,message,data}` 包装。刷新请求为可选 `exchange` 与 `code`，空请求等价于全股票市场刷新，股票身份和 JSON 严格校验保持原契约。

workbench 在有限的刷新客户端边界内转发经过校验的 DTO；不建立任意路径代理。NAS 完成证券精确解析与运行中检查，并调用原应用服务。全市场请求仍只触发股票调度，期货依旧由现有独立调度控制，避免扩大原接口语义。

| 条件 | 对工作台调用方的响应 |
|---|---|
| 单证券刷新成功 | 200，原 RefreshResult 字段 |
| 全市场触发成功 | 202，`data.status=ACCEPTED`；只表示已接受 |
| 非法请求、证券不存在、身份歧义、重复刷新 | 保留 400/404/409/429 及原稳定 message |
| 无法连接更新服务 | 503 `UPDATER_UNAVAILABLE` |
| 等待更新服务超时 | 504 `UPDATER_TIMEOUT` |
| 内部 Token 被拒绝、上游异常/不合法响应 | 502 `UPDATER_BAD_RESPONSE`，不透传正文或认证细节 |

内部接口 Token 缺失或错误返回 401 `UNAUTHORIZED`；工作台不将内部 401 作为浏览器登录问题暴露。未知内部 5xx 不直接转发任意错误文本。

请求与响应体均限制 1MiB；刷新客户端总超时 40 秒，禁止自动重试 POST、禁止跟随重定向，避免重复触发或泄漏 Token。仅发送固定服务地址和固定接口所需的头，不转发浏览器 Cookie/Authorization。配置 URL 仅接受 http/https 源地址，不允许 userinfo、query、fragment 或额外路径。

单证券调用沿链路传递请求取消；全市场成功受理后使用 updater 根 context，电脑退出或断网不取消已接受刷新。超时/断线可能发生在 NAS 已接受或提交之后，因此响应不得承诺“未执行”，也不自动重发。

## 4. 配置与构建

保留现有 `Config.DB`、`Config.Worker`、`Config.Market` 字段和默认值；新增：

| 配置 | 用途与规则 |
|---|---|
| `Config.Server.ListenAddress` | updater 默认 `:8081`，workbench 默认 `127.0.0.1:8080`；容器工作台示例显式使用 `:8080`，宿主只映射回环地址 |
| `Config.Updater.URL` | workbench 必填，用于固定刷新服务地址；updater 不依赖此项 |
| `Config.Updater.Token` | 两服务必填且相同的非空共享随机 Token；至少 32 字节，作为 Bearer 认证，比较避免时序泄漏；只放本地配置 |

updater 使用 `Worker.ScanBatchSize` 作为原行情并发配置，避免借拆分改变配置含义；workbench 继续按原 Worker 配置运行扫描和回测。各角色只校验与自身装配有关的配置，但服务角色和必需的连接/认证配置先校验再开始工作。

提供 updater/workbench 两份无真实地址、凭据的配置样例。Dockerfile 提供 `updater`、`workbench` 两个构建目标；NAS updater 镜像不构建/携带前端，workbench 镜像含前端产物。两个目标均以非 root 运行，具有 CA 与时区数据，只读挂载配置，按 linux/amd64 验证。

NAS Compose 示例只启动 updater，设置 `restart: unless-stopped`，连接已有 MySQL，不新建数据库容器或数据卷。升级时先停止旧 updater 再启动新实例，不使用多副本滚动发布。本机提供两个明确的运行/构建命令，并更新旧单体命令及验证脚本。

## 5. 生命周期、故障与安全

- 每个数据库只部署一个 updater，所有定时和手动刷新共用该进程的守卫与限频器。部署边界保证唯一发布者；不引入跨进程锁，也不宣称误部署两个 updater 仍安全。
- updater 停机取消根 context，等待 HTTP、调度和已接受手动任务退出后关闭数据库；workbench 停机等待 HTTP 和计算 Worker 退出后关闭自己的连接。
- workbench 启动不要求 updater 在线。数据库可达且 schema 可用时，查询、指标、计算继续工作；刷新失败按上节明确返回，不回退本地采集。
- MySQL 不可达时不会改读 mock 库。启动连接失败则退出；运行期沿现有数据库错误、租约和任务恢复规则处理，不增加离线队列或离线浏览。
- 电脑关闭期间扫描/回测不继续计算，已持久化任务按现有租约恢复逻辑处理；NAS 不接管计算任务。
- 本次只支持可信局域网。NAS MySQL 和刷新端口仅允许所需局域网连接；工作台默认只供本机浏览器访问。Token 不写入镜像、日志、前端或 Git。
- 外网是后续需求，保留可配置的地址和 http/https 支持。本次不开放公网端口，也不部署 VPN 或额外网关；远程接入方式另行设计。

## 6. 切换与回滚

1. 备份 NAS 数据库，核对现有 schema/版本及两服务配置；不对真实业务库运行测试。
2. 停止旧单体，避免行情双写；以同一代码版本启动 updater，完成现有 schema 初始化。
3. 启动电脑 workbench，确认历史图表可读，工作台写入与扫描/回测正常，刷新由 NAS 执行。
4. 关闭电脑 workbench，验证 updater 仍更新；停 updater，验证工作台仍读已完成数据且刷新返回明确错误。
5. 如需回滚，先停两服务，再恢复旧版本单体及旧配置。本次无表结构/数据格式变更，保留数据库；若另行执行数据库迁移，按对应迁移的回滚规则处理。

这里的回滚使用旧发布版本，不代表新版本保留 all 模式。

## 7. 方案比较

| 方案 | 收益 | 成本与选择 |
|---|---|---|
| 共享 MySQL，仅刷新走 HTTP（本方案） | 复用现有事务、版本和批量读取，改动集中 | 两服务需协调 schema 版本；适合当前单用户局域网 |
| 行情全部经 NAS HTTP 数据接口读取 | 服务数据边界更独立，客户端不直连 MySQL | 需增加批量、版本、预热与分页传输契约；本次无必要 |
| NAS 向电脑复制行情 | 可支持离线查看 | 需处理同步进度、修订、一致性及存储；本次不采用 |

## 8. 测试与当前文档回填

先补失败测试，再实现角色装配、刷新协议、初始化归属与部署构建。验证角色缺失/非法、没有隐式 all、工作台不启动采集、updater 不领取计算任务、独立取消/退出、上游超时/异常/Token/大小限制、共享库读写及 COMPLETE 可见性。

保留现有行情、指标、策略、回测与前端测试。执行 `bash scripts/verify.sh --full`；两种镜像目标都需实际构建，MySQL 测试只用获批远端 8.4/x86_64 随机隔离库。总覆盖率至少 80%，核心领域各至少 90%；完成后由独立子 Agent 评审。

实现时同步回填系统设计、API 设计与 HTTP 契约、应用层、端口、data、MySQL、工程标准、运行手册及源码地图。新增包逐一分配唯一 owns，旧组合根中失去用途的采集/Worker 装配代码清理；不修改用户未提交的界面工作。本次部署决策由该变更记录完整说明，不另建重复 ADR。

## 9. 批准范围与证据

用户已批准提交 `03abaed` 中 [需求](requirements.md) 与本设计，覆盖共享 MySQL、内部刷新 HTTP、schema 初始化归属、CLI/配置/镜像和局域网边界。

用户于 2026-09-21 在本任务回复“ok 执行吧”，批准提交 `03abaed` 中需求与设计的完整范围；审批元数据记录该决定。

## 本次交付验收调整（用户明确授权）

实施期间用户确认“现在连不上 nas。代码编译成功就行”。该决定覆盖本次 REQ-SPLIT-010 / 设计第 8 节中的外部验收前置要求：本次以 Go 本机编译、linux/amd64 交叉编译及前端构建作为交付门槛，不等待真实 NAS/MySQL、Docker 镜像构建或现场部署。已经执行的测试如实记录，不把缺失验证标为通过；独立代码评审继续执行。允许按此范围完成并合回 main。服务运行设计和其他变更的通用工程门禁不因此改变，NAS 实际部署另待环境恢复。
