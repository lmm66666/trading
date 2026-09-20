---
id: CHG-2026-09-21-board-persistence
status: implementing
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21"
approved_revision: "sha256:c563adbfdf29e0fc1c43afba894d8fc8bf811b61794fbbba5020390aadea1e91"
approved_scope: [PERSIST-001, PERSIST-002, PERSIST-003, PERSIST-004, PERSIST-005, PERSIST-006]
---

# 看板服务端持久化

## 问题与用户场景
看板目前只存浏览器 localStorage，更换浏览器或清理站点数据即丢失。看板是行情工作台的核心配置，应随服务端数据长期保存。个人研究工具、单用户、无账户体系；用户在任一浏览器登录同一后端即可看到相同看板。

## 范围与使用条件
- 看板归属为单用户全局：表不加 owner 字段（用户已裁决）。
- 不迁移 localStorage 既有数据，从默认看板开始（用户已裁决：尚未使用看板，本地无数据）。
- 手动保存、低频写入：每次显式操作恰好一次请求；最多 20 个看板、每看板配置约 1KB；列表全量返回，无轮询。

## 需求
- `PERSIST-001` 服务端存储：看板名称与完整配置持久化到 MySQL 新表 `t_chart_boards`，沿用 `t_watchlist` 单用户表范式（无符号自增主键、无用户列、上限由应用层校验）；数据库成为唯一数据源，重启或更换浏览器后看板一致。
- `PERSIST-002` 看板 API：新增五个接口——`GET /api/v1/chart-boards` 全量读取；`POST /api/v1/chart-boards` 创建（创建即激活）；`PUT /api/v1/chart-boards/:id` 更新名称和/或配置；`POST /api/v1/chart-boards/:id/activate` 激活；`DELETE /api/v1/chart-boards/:id` 删除。所有变更接口成功后返回更新后的完整看板状态（boards + active_id），不引入二次拉取；错误沿用稳定 message：400 INVALID_REQUEST、404 NOT_FOUND、409 BOARDS_FULL（超过 20）、409 LAST_BOARD（删除最后一块）。
- `PERSIST-003` 服务端校验：写入路径严格校验，口径与现有前端一致——名称 trim 后 1–40 字符；config 拒绝未知字段；defaultSymbol 为空或完整 A 股身份格式；timeframe DAY/WEEK、priceView RAW/QFQ；指标 0–16 个、复用图表指标参数校验且参数身份唯一；comparison 为空或八个既有期货连续品种之一；visibleBars 10–400；paneWeights ≤18 键且值域 (0,10000]。读取路径原样返回已存 JSON，不重复校验。
- `PERSIST-004` 激活看板：当前激活看板由服务端持久化（恰一激活语义由事务维护）；重进恢复上次选定看板及其默认股票，显式 URL 股票优先；删除激活看板时服务端激活剩余 id 最小者；空表时首个创建的看板自动激活。
- `PERSIST-005` 前端改造：`useBoards` 改为服务端 API 驱动，移除 localStorage 读写（`wb.boards.v1` 弃用不清理）；看板列表为空时前端显式创建"默认看板"；看板加载完成后若 App 尚未选定证券且激活看板有默认股票则自动选中；无 URL 股票时初始股票等待看板加载解析；操作进行中禁用重复提交并保留草稿；加载/保存失败显示可理解错误并可重试；保存/切换/新建/重命名/删除成功后以服务端返回的完整状态替换本地状态。
- `PERSIST-006` 并发语义修订：以服务端为权威，同源多标签页按操作级最后写入生效，各操作成功后以响应全量刷新本地状态；移除原"保存前对比存储快照拒绝覆盖"的冲突检测（修订 CHG-2026-09-20-chart-boards 中 BOARD-005/006 的相应条款，其余看板交互语义保持）。

## 验收标准
- Go 契约测试覆盖五接口的成功路径、全部错误码、strictJSON（未知字段/尾随 JSON/嵌套 config）与全量返回结构；MySQL 随机隔离库集成测试覆盖建表、CRUD、激活迁移、删除激活行与末板保护。
- 前端测试覆盖：加载、空表建默认看板、保存/另存/重命名/删除/切换（含"保存并切换"两步操作）、失败保留草稿、busy 禁用、初始股票解析与 URL 优先。
- `bash scripts/verify.sh --mysql` 通过；`--image` 记录为不适用（无构建、依赖或部署变化）。

## 非目标
不做多用户/账户体系、不做看板导入导出或版本历史、不做自动保存、不迁移 localStorage 旧数据、不扩展期货白名单、不改图表查询/指标/策略/回测语义、不做跨设备实时同步推送。

## 兼容与开放问题
- 语义修订见 `PERSIST-006`；原 localStorage 键不再读取。
- 无存量数据迁移；新表由 AutoMigrate 幂等创建，回滚即停机删表。
- 前端与后端各自维护 comparison 白名单（8 项）与校验口径，服务端为权威；扩展品种需同步两处（已接受的重复成本）。

## 批准范围与证据
用户于 2026-09-21 批准全部需求 PERSIST-001…006，含 PERSIST-006 对 CHG-2026-09-20-chart-boards BOARD-005/006 多标签页冲突条款的语义修订；内容版本 `sha256:c563adbfdf29e0fc1c43afba894d8fc8bf811b61794fbbba5020390aadea1e91`（批准字段为空时计算）。配套设计见 design.md。
