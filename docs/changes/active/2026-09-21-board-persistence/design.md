---
id: CHG-2026-09-21-board-persistence-DESIGN
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21"
approved_revision: "sha256:5d125bf7a9161ed2d4e12e6258122a6903b81f051c97fe8ca1343b7ab3874257"
approved_scope: [PERSIST-001, PERSIST-002, PERSIST-003, PERSIST-004, PERSIST-005, PERSIST-006]
---

# 看板服务端持久化设计

## 现状与目标
现状：看板整存整取于 localStorage `wb.boards.v1`（schemaVersion=1），读取时严格校验，写入前对比存储快照拒绝并发覆盖；App 另行同步读取以解析初始股票。目标：MySQL 新表 + 五个 HTTP 接口成为唯一数据源；前端 `useBoards` 异步 API 化；并发语义改为服务端权威、操作级最后写入。

## 数据模型与存储
- 新表 `t_chart_boards`：`BaseModel`（无符号自增 id、UTC `DATETIME(6)` 时间戳）+ `name VARCHAR(40) NOT NULL` + `config JSON NOT NULL` + `is_active TINYINT(1) NOT NULL DEFAULT 0`。允许同名（无业务唯一键，沿用现状）；列表按 id 升序即插入顺序；上限 20 由应用层校验（`t_watchlist` 范式）。
- `migrationModels` 追加 `ChartBoardModel`（无必检索引，迁移后仅验证表存在）。
- config 落库为应用层校验后的规范化 JSON 文本：写路径是唯一入口（严格解析→校验→按 DTO 序列化存储）；读路径原样透传，不重复校验。MySQL JSON 列保证语法合法；规则演进只约束新写入，不影响旧行读取。

## 端口与应用层
- `internal/port/chart_board.go`：`MaxChartBoards = 20`；`ChartBoard{ID uint64, Name string, Config string}`（Config 为规范化 JSON 文本）；`ChartBoardState{Boards []ChartBoard, ActiveID uint64}`；`ChartBoardStore` 接口提供 `List/Create/Update/Activate/Delete`，全部返回 `ChartBoardState`。端口不依赖应用层类型。
- `internal/application/chart_board_service.go`：编排校验与上限——名称与 config 校验（指标部分复用 `validateIndicatorRequest` 与图表口径）；数量 ≥20 → `ErrBoardsFull`；仅剩一块时删除 → `ErrLastBoard`；未知 id 传播存储层 `ErrChartBoardNotFound`；变更后返回完整状态。
- `application.ChartBoardConfig`（typed DTO，JSON 字段名与前端 `BoardConfig` 完全一致）：`defaultSymbol *string`、`timeframe`、`priceView`、`indicators []IndicatorRequest`、`comparison *string`、`paneWeights map[string]float64`、`visibleBars int`。
- config 校验规则（与前端 `boards.ts` 同口径）：defaultSymbol 为空或 `^(SSE|SZSE|BSE):[A-Z0-9]{1,32}$`；timeframe/priceView 枚举；指标 ≤16、参数复用图表校验（SMA/EMA/KDJ/STD period 1–500 且其余为 0；MACD period=0、fast≥1、slow>fast 且 ≤500、signal 1–500）、`(kind,period,fast,slow,signal)` 身份唯一；comparison 为空或白名单 {INE:SC.MAIN, SHFE:FU.MAIN, INE:LU.MAIN, SHFE:AU.MAIN, SHFE:AG.MAIN, DCE:J.MAIN, DCE:JM.MAIN, CZCE:ZC.MAIN} 之一；visibleBars 10–400 整数；paneWeights ≤18 键、值为有限数且 ∈ (0,10000]。保存路径不做指标成本预算检查（与现 localStorage 行为一致，预算仍由图表查询执行时校验）。

## 存储事务（is_active 恰一不变量）
- Create：事务内 INSERT 新行（is_active=1）后将其余行置 0；空表时该行自然成为唯一激活。
- Activate：事务内先条件 UPDATE 目标行置 1（affected=0 → NotFound），再将其余行置 0。
- Delete：事务内删除（affected=0 → NotFound）；若剩余行中无激活行，则激活 id 最小者。
- Update：单条条件 UPDATE（name 和/或 config），不触碰 is_active。
- 全部参数化、显式列名；事务只包围上述语句，不调用外部 HTTP 或长计算。

## HTTP 契约（完成后回填 http-api.md）
- `GET /api/v1/chart-boards` → 200 `{"boards":[{"id","name","config"}...],"active_id"}`，id 升序。
- `POST /api/v1/chart-boards` body `{"name","config"}` → 200 全量状态；创建即激活。
- `PUT /api/v1/chart-boards/:id` body `{"name"?,"config"?}`（至少一项）→ 200 全量状态。
- `POST /api/v1/chart-boards/:id/activate` → 200 全量状态。
- `DELETE /api/v1/chart-boards/:id` → 200 全量状态；若删除激活看板，服务端激活剩余 id 最小者。
- 错误映射：JSON/字段/校验 400 INVALID_REQUEST；未知 id 404 NOT_FOUND；超上限 409 BOARDS_FULL；删除最后一块 409 LAST_BOARD。请求体上限 1MiB、拒绝未知字段与尾随 JSON（strictJSON 递归覆盖嵌套 config 与指标对象）。
- 实现归属：api 五个独立 handler 文件 + `KernelServices.ChartBoards` 接口；config 以 `json.RawMessage` 透传；`main.go` 装配 `NewChartBoardStore` + `NewChartBoardService`。

## 前端改造
- `api/client.ts`：`BoardConfig` 类型迁入（成为 API DTO，`boards.ts` 反向引用）；新增 `listChartBoards/createChartBoard/updateChartBoard/activateChartBoard/deleteChartBoard`；不做 dev mock 回退（与自选一致）。
- `boards.ts`：删除 `BOARDS_KEY/readBoards/saveBoards/validStore/BoardStore`；保留 `comparisonOptions`、`defaultBoardConfig`、`validIndicator/validConfig` 作为提交前预检（服务端为权威）；`Board.id` 改为 number。
- `useBoards.ts`（仍挂载于 ChartWorkspace，App 不重复请求）：挂载 GET；空列表时 POST `defaultBoardConfig()` 建"默认看板"，此时草稿 = 默认配置 ∪ URL 参数（保持现首建行为，此后 URL 参数不再种子草稿）；加载完成后若激活看板有 defaultSymbol 则触发 onSelectSymbol（App 仅在尚未选定证券时接受，保证 URL 股票优先）；`save`=PUT config、`select`=（可选先 PUT 当前 config）+ POST activate、`create`=POST、`rename`=PUT name、`remove`=DELETE；成功以响应全量替换本地状态并同步草稿；busy 期间禁用变更操作；失败保留草稿并显示服务端 message；dirty/beforeunload/restore 语义不变；加载失败提供重试。
- `App.tsx`：删除 `readBoards` 同步读取；初始股票 = URL 股票，否则等待看板加载回调；看板加载中沿用未选证券的欢迎态。
- `BoardToolbar.tsx`：异步处理与 busy 禁用（含"保存并切换"两步操作失败时保留对话框）；提示文案"看板保存在当前浏览器"改为"看板保存在服务端"。

## 失败、降级与恢复
- 后端不可达：加载失败显示错误与重试；操作失败保留草稿与本地状态，不部分应用。
- 服务端 4xx：按稳定 message 显示对应中文说明；4xx 后本地状态不变。
- 看板数据只经写路径进入，读取不校验；若数据库行被外部破坏，错误按 500 脱敏处理。

## 并发、容量与安全
- 单用户低频手动写；20 行 × ~1KB，全量响应 <50KB；无轮询。
- 多标签页：操作级最后写入生效，各操作响应全量刷新本地；空表首载两个标签页并发建默认看板可能产生两块同名看板（后建者激活），个人工具接受，可重命名/删除清理。
- is_active 恰一由事务维护；单实例部署无跨进程争用。
- config 不含凭据或行情数据；日志不记录请求体；SQL 参数化、显式列名。

## 兼容、迁移与回滚
- 新表由 AutoMigrate 幂等创建；无数据迁移（用户裁决）；`wb.boards.v1` 不读取不清理。
- 回滚：停机删除 `t_chart_boards`（无其他表引用）并回退前端版本。
- 语义修订：CHG-2026-09-20-chart-boards 的 localStorage 存储与多页冲突拒绝条款由本变更覆盖；手动保存、末板不可删、草稿分离、URL 优先等交互语义保持。

## 测试策略
- 测试先行。Go：application 服务校验矩阵/上限/末板/NotFound/状态组装；api 五接口 strictJSON（含嵌套 config 未知字段）/错误映射/DTO/未装配；mysql 集成（--mysql 门禁）：建表、CRUD、激活迁移、删除激活行、末板保护。
- 前端：useBoards 全流程（加载/空表建默认看板/保存/另存/重命名/删除/切换含两步操作/失败保留草稿/busy/初始股票回调）、boards 保留校验、BoardToolbar 文案与禁用、App 初始股票、client 新函数。
- 门禁：`bash scripts/verify.sh --mysql`；`--image` 记录 not-required（无构建、依赖或部署变化）。

## 影响的当前文档
`docs/design/web.md`（§10 看板存储）、`docs/standards/http-api.md`（新看板接口章节）、`docs/design/api.md`（§2/§8）、`docs/design/internal/application.md`（看板服务小节）、`docs/design/internal/port.md`（新端口）、`docs/design/internal/infrastructure/mysql.md`（新表小节）、`docs/design/README.md`（功能目录相应行）。

## 备选方案
- 整存整取单行 JSON（保留 BoardStore 形状 + 乐观锁）：可完整保留并发冲突检测，但引入单行锁语义且与 watchlist 范式不一致；单用户低频场景收益低，弃。
- 服务端不存激活看板（前端每次默认第一块）：少一列但丢失已批准的"重进恢复上次选定看板"行为，弃。
- config 读取时重复严格校验：规则收紧会使旧行不可读；写路径为唯一入口已足够，弃。
- App 独立 GET 解析初始股票：双请求且与工作台状态可能瞬时不一致；改为加载回调上抛（onSelectSymbol），单请求单数据源。

## 批准范围与证据
用户于 2026-09-21 批准本设计全部范围（PERSIST-001…006）；内容版本 `sha256:5d125bf7a9161ed2d4e12e6258122a6903b81f051c97fe8ca1343b7ab3874257`（批准字段为空时计算），与 requirements.md 同次批准。
