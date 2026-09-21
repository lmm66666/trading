---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["internal/application/"]
related: []
---

# 应用层设计

想先理解扫描结果、时间窗口和数据流，请读 [策略扫描流程](../workflows/strategy-scan.md)。本文保留应用层的依赖、执行和生命周期契约；扫描说明中已列出预热表述与实现的待裁决差异。

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `internal/application` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

应用层编排行情采集与查询、图表、证券搜索、扫描、回测、持久化 Worker 和调度生命周期。它负责固定可复现输入、协调端口、传播取消、分类重试，并把领域结果交给原子发布边界。

应用层不实现指标、策略、撮合、外部协议解析或数据库事务；这些职责分别属于领域模块和适配器。

## 2. 对外能力与使用者

- API 使用行情查询、图表、证券搜索、自选清单、回测和扫描服务。
- updater 组合根使用行情采集与股票/期货调度器；workbench 组合根使用持久化 WorkerPool。
- 服务输入输出只使用领域值对象、应用 DTO 和 `internal/port` 契约，不暴露 ORM Model。

## 3. 依赖和边界

应用层只依赖 `market`、`indicator`、`strategy`、`backtest` 与 `port`，不导入 Gin、GORM、具体 Broker 或旧 K 线模型。业务算法留在领域模块，外部协议解析留在适配器，事务与并发写入仲裁留在仓储。

可选扩展端口通过能力检测使用；缺少增量变化分类时安全全量计算，不能用不完整包装器隐藏生产仓储已经具备的幂等读取或变化分类能力。

## 4. 核心模型与不变量

- 回测和扫描创建时固定策略定义、参数、窗口、证券集合、行情版本和引擎版本，并以 canonical SHA-256 保护完整输入；执行前重新核验保存输入和摘要。
- 扫描为每个证券创建独立策略实例，结果按证券稳定排序；后续分页始终绑定首次返回的 SnapshotID。
- 暖机行情只用于指标预计算，账户和信号窗口从请求起点开始，禁止读取未来数据。
- 行情刷新只消费 `port.DailyMarketSource` 的日线与复权因子；周线由应用层对最终日线调用 `market.AggregateWeekly` 确定性生成。
- 当前生产刷新不获取公司行动。写入是增量 upsert，缺省 Bar、因子或公司行动不表示删除；端口尚无权威快照撤回语义。
- DTO 时间使用 UTC；跨持久化身份按微秒规范化，与 MySQL `DATETIME(6)` 一致。

## 5. 主要流程

### 5.1 回测、扫描与 Worker

创建回测时锁定一次最新 COMPLETE 行情版本；执行阶段先校验数据质量、归属和版本，再计算指标与运行引擎。取消、引擎错误或失效租约均不能发布结果。

扫描最多固定 5000 个证券，通过一次批量读取加载主/辅助周期。只有同一 AsOf、配置、证券集合和前快照身份可以增量复用；纯 Bar 修订只重算 dirty 证券，因子修订或公司行动修订使整个快照失效。缺少变化分类能力时全量重建。

幂等提交先读取已存在 Run，再由数据库唯一约束仲裁并发首次提交。碰撞后重读胜出者，核验其完整保存输入和摘要；相同键但用户输入不同返回 `port.ErrIdempotencyConflict`，其他存储错误不得伪装成碰撞。

WorkerPool 使用固定 worker、周期 reaper 和活跃任务续租。续租周期为租期三分之一；持久 Attempts 决定 250ms、1s、4s 退避，第四次失败进入终态。应用关闭先取消根 context、等待 WorkerPool 和调度器退出，再关闭数据库。

### 5.2 行情刷新

首次刷新读取 HistoryStart 至当前 UTC 的日线；普通股票刷新从最近 20 根已存日线起点重抓，期货可由配置选择全历史刷新。来源日线与已存数据合并后生成周线，再获取并规范化复权因子，校验范围、重复时间、因子覆盖和最终日周一致性，最后计算 canonical 批次并一次发布。

当前生产装配把同一 `rate.Limiter` 注入股票和期货来源适配器，在 HTTP Transport 边界共享限频；`MarketIngestionConfig.Limiter` 是额外的可选并发协调钩子，生产配置目前不注入。两者不能在设计中混写成已经启用的同一机制。

同一证券的并发刷新在单进程内返回 `ErrRefreshAlreadyRunning`。多进程部署仍必须保持单个行情发布者；当前没有跨进程刷新租约或基线版本 CAS。

### 5.3 行情查询与调度

行情查询的零版本只解析一次最新 COMPLETE 版本，之后始终读取该确切版本。零 To 默认当前 UTC，零 From 默认向前 20 年，零 Limit 默认 5000；返回最近 Limit 根并按收盘时间升序。复权查询缺因子或版本不符直接失败。

调度器使用 1–64 个固定 worker，范围最多 5000 个去重证券。`Start` 启动后先等待 interval，首个 tick 到达才执行首轮，后续按原 ticker 同步循环；逐证券失败进入汇总但不终止后续周期，全局读取或生命周期错误才结束循环。

### 5.4 自选清单

`WatchlistService` 编排自选三接口：`List` 读取条目后按证券行 ID 批量读取最新日线报价并组装 DTO，无报价条目对应字段为 null；`Add` 校验身份（非法 400）与活跃性（未知或非活跃 404）、超过 `MaxWatchlistItems`（100）返回上限错误（409）后幂等写入；`Remove` 幂等删除。变更接口返回更新后的完整列表。报价由 MySQL 仓储单条窗口函数 SQL 批量完成，应用层不按证券循环查询。

### 5.5 行情看板

`ChartBoardService` 编排看板五接口，单用户全局（无 owner 维度）：名称校验 1–40 字符（首尾空白剔除）；config 以 `json.RawMessage` 接收后严格校验（defaultSymbol、timeframe/priceView 枚举、指标 ≤16 且身份唯一、comparison 白名单、visibleBars、paneWeights）并规范化序列化落库，读路径原样透传。超过 `MaxChartBoards`（20）返回上限错误，删除最后一块返回 `ErrLastBoard`；删除激活看板时激活剩余 id 最小者，恰一激活不变量由存储事务维护。所有变更成功后返回全量状态（boards 按 id 升序 + active_id），不做跨标签页冲突检测。

## 6. 失败、取消和一致性语义

- context 取消贯穿端口、领域 Replay、Worker 和调度器；取消或失租不重试、不发布。
- 只有明确 temporary、timeout 或 transient connection 错误可重试。落库 `ErrLeaseLost` 只结束当前任务，其他落库错误传播并终止受影响生命周期。
- 行情空响应、已知日期丢失、重复时间、非法因子或最终日周不一致均返回 `ErrIncompleteMarketData`，任何一步失败都不发布部分批次。
- 全局数据版本由 writer 原子分配；应用层预检查不能替代数据库唯一约束和条件更新。

## 7. 性能与安全约束

- 时间范围最多 20 年，单次 Bar/因子上限使用 port 的 10000 约束；扫描证券最多 5000，所有 worker 数有界。
- 批量场景调用批量 port，不按证券产生无界数据库往返或 goroutine。
- `SlogTelemetry` 只接收声明的阶段、身份、版本、耗时和计数字段，不记录请求体、配置或原始错误文本；当前 `queue_wait` 只表示 Claim 调用等待。

## 8. 测试与验收证据

```bash
go test -race ./internal/application -run 'Test(Refresh|Prices|MarketScheduler)'
go test ./internal/application ./internal/port -cover
go test ./...
go vet ./...
```

测试覆盖输入摘要、固定版本、增量扫描、租约与取消、批量上限、日线合并、本地周线生成、复权因子覆盖、可选 Limiter、调度生命周期、自选校验/报价组装与幂等语义、看板校验/上限/末板保护与全量状态语义。

## 9. 相关文档

- [系统设计](../../architecture/system-design.md)
- [领域地图](../README.md)
- [应用端口设计](port.md)
- [行情领域设计](market.md)
- [MySQL 适配器设计](infrastructure/mysql.md)
- [Broker 设计](../pkg/broker.md)


## 拆分后的生命周期

每个目标库单个 updater 持续采集；workbench 只执行查询、工作台操作和扫描/回测。两者各自取消并等待所拥有的后台任务退出，最后关闭自身数据库连接。workbench 的刷新请求通过 HTTP 交给 updater，已受理的全市场任务属于 updater 根 context。工作台停机不取消 NAS 更新；计算任务沿用数据库租约恢复，NAS 不领取计算任务。MarketScheduler、FuturesScheduler 启动不立即采集，首个周期后才自动执行；手动股票刷新可立即触发且不重置定时节拍。周期、范围、限频和发布算法不变。

## 双价格 Z-score

ChartQueryService 的 comparison 限已有八项国内期货。`ZSCORE(period,smooth,regime,lag)` 在相同正版本读取股票、商品完整历史，商品仅读取一次。按UTC日期取不晚于股票日期的最近已知商品Bar，再滞后lag根商品Bar；股票RAW/QFQ、商品RAW，计算后按股票页裁剪。历史下界固定为服务UTC当前时刻向前20年，不随分页before移动；边界之前返回空页。

参数 band2–500、smooth1–500、regime>band且≤500、lag0–5；其他类型不得带smooth/regime/lag。成本 `2*period+regime+68` 含诊断，仍受2000/16/并发4约束。三分量与其他指标按请求顺序返回；key包含两腿身份、复权及所有参数。

未选关联、周线、商品缺历史/读取失败/版本不符只返回ZSCORE诊断warning；股票和其他指标保留。取消和超时继续传播。zscores返回最新股票点上的主Z、长期Z、63期相对表现、5期收益相关、商品日期、lag和状态，未定义字段为null。旧STD/RETZ不再接受新增/保存；读取看板不改库，客户端修正草稿后手动保存，复用原表/SQL。

## 批量刷新进度

`RefreshProgress`、`RefreshObservation` 记录准备、单证券完成与终态事件，`RefreshQueries` 聚合当前股票/期货摘要。股票/期货 scheduler 只增加观察事件，不改变范围、guard、限频、增量窗口或行情写入。已处理=成功+失败；取消和未派发项不计作证券采集失败，终态中断保留最后已处理计数；无变化的成功也计成功。

每 10 秒及状态转换时保存最新绝对计数快照；保存按观察任务串行、带单调 revision，失败继续采集并重试最新观察快照。每次数据库观察操作上限 2 秒，退出刷新总上限 5 秒。pending 上限 64，超限仅禁用新任务观察并告警，不阻断采集。启动恢复固定截止时刻并重试，避免误中断新任务。进度存储和行情事务独立，最后记录可以落后实际行情，不能据此恢复断点。
