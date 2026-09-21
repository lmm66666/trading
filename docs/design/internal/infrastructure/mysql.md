---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["internal/infrastructure/mysql/", "internal/infrastructure/mysql/dbtest/"]
related: []
---

# 策略内核 MySQL 存储设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `internal/infrastructure/mysql` |
| 最后更新 | 2026-09-20 |

## 1. 职责与非职责

本模块实现版本化行情、证券目录、自选清单、行情看板、持久化任务与租约、回测结果、扫描快照、Outbox 和旧行情迁移所需的 MySQL 端口。它负责数据库事务、并发仲裁、索引和 GORM/SQL 映射，不定义策略、指标或撮合业务规则。

本模块不定义策略、指标、撮合、任务重试策略或 HTTP 行为。应用和领域模块不得依赖本模块；生产由组合根把适配器注入 `internal/port` 定义的边界。

## 2. 对外能力与使用者

- 组合根使用迁移与构造函数初始化数据库。
- 应用层通过 port 使用版本化行情、证券目录、自选清单、行情看板、任务队列、运行结果、扫描快照和 Outbox。
- 旧行情迁移命令使用专用读取、暂存、检查点和最终切换能力。
- `data` 包通过现有兼容初始化桥接复用本模块连接；该依赖不应扩展到新领域代码。

## 3. 依赖和边界

本目录提供 MySQL 8.4 的版本化行情仓储和内核表结构。一张表一个模型，领域及 port 不依赖 GORM。连接必须使用 `parseTime=true&loc=UTC`；时间以 UTC `DATETIME(6)` 存储，Price/Money 使用有符号 BIGINT，版本和序号使用无符号整数。

本模块依赖 `port`、`market` 和 `backtest` 完成端口映射；对根 `model` 的依赖只服务财报兼容与旧行情迁移。它不得调用外部 HTTP，也不得在数据库事务中执行领域长计算。

## 4. 核心模型与不变量

不透明身份字段采用 VARBINARY，保证 `Key`、`key`、`key ` 精确区分，避免 `_ci` 排序规则及 PAD SPACE 的隐式合并。RunID、SnapshotID、EventID、StrategyID、摘要/参数 hash 为 64 字节；策略/引擎版本为 32 字节；幂等键、租约 owner/token、AggregateID、Source、SourceEventID 为 128 字节。`port.ValidateIdentity` 及各 DTO Validate 校验相同的 UTF-8 字节上限，不 trim 或修改身份；接受标量身份参数的适配器也必须调用该验证。普通状态、类型、名称、原因保持文本。当前 OrderID 把无长度限制的 Reason 拼入标识，OrderID/FillID 因而使用不参与索引的 LONGBLOB，保持字节完整。

连接初始化在所有迁移完成前持有所有权，旧表或内核迁移任一步失败都释放连接，成功后才交给 Data。若已有早期测试数据，切换身份列前需确认其字节长度符合端口边界，不能依赖数据库截断修复超长旧值。

版本 0 仅是内部发布锁；业务版本必须大于 0 且为 COMPLETE。历史可见区间固定为 `[valid_from_version, valid_to_version)`。市场写入是增量 upsert，未提供的数据表示未观测而不是删除。

租约所有权由 run、owner、随机 token 和数据库时间共同确认；失效 worker 不得发布、重试或覆盖新领取者。扫描分页始终绑定精确 SnapshotID，不能在续页时重新选择最新快照。

## 5. 主要流程

### 5.1 初始化、行情发布与读取

`Migrate` 按依赖顺序创建 17 张内核表，检查关键索引，并幂等建立版本 0 锁行。版本 0 的状态为 `INTERNAL_LOCK`、来源为 `__kernel_version_lock__`，只用于序列化发布，永远不能作为已完成数据版本读取。updater 正常启动仅迁移证券主数据和新内核表；旧技术 K 线表只供迁移或回滚读取，不创建、不变更。

`Publish` 是增量 upsert：未提供的 Bar、因子或事件表示本次未观测，不表示删除；当前端口没有权威快照或删除语义。批次先复制、排序、校验并计算 SHA-256；提供的 Digest 必须匹配 `MarketBatchDigest`。输入版本号不参与摘要，发布版本由仓储分配。仅该证券最近一条 COMPLETE 发布可命中摘要幂等，重新提交较老内容会形成新版本。

发布事务锁定内部锁行，按 `(exchange, code)` 解析证券，写入 PENDING 版本，只关闭发生变化的当前修订，批量插入替代修订，最后标记 COMPLETE。首次遇到证券时，未知 Name/Board 保留空串，Active 为 false、LotSize 为 0；后续主数据同步负责补齐并激活。事件 ID 在单个证券内必须由上游保证来源唯一，使用 VARBINARY 精确保存大小写及尾空格，修订键包含版本以保留历史。

COMPLETE 表示已提供变更全部提交。当前写入端口没有上游质量字段，因此不会根据“没有公司行动/没有因子”推断真实数据缺失。数据版本 Quality 记录 COMPLETE；需要复权数据的消费者必须检查对应时段因子是否可用，不能把该质量标志当成收益可信证明。

请求版本必须已完成且大于 0。历史可见区间为 `[valid_from_version, valid_to_version)`；批量读取在只读 Repeatable Read 事务内完成，最新版本可以安全使用 current 索引条件。Dataset 按请求版本重新标记和校验；单个证券的非法数据只产生该证券错误，数据库读取失败则显式返回所有受影响证券的错误。

`BatchDatasets` 最多接受 5000 个证券，基础读取固定 6 条 SELECT：请求版本、最新版本、证券、所有周期 Bar、因子、公司行动。`LookbackBars` 表示 From 前最多 N 根 Bar；通过有 LIMIT 的参数化 UNION ALL 读取，最多再加 2 条 SELECT。每条最多 7500 个分支、45000 个参数，低于 MySQL 65535 参数上限。不会将所有历史 Bar 搬到应用层裁剪；长 UNION 的解析成本和查询包大小仍需真实环境压测。调用者应控制窗口、标的数及暖机根数，避免请求本身产生过大结果集。

`DirtyInstruments(after, through)` 合并 Bar、因子、公司行动在 `(after, through]` 内的新修订，使用三组 `(valid_from_version, instrument_id)` 索引，去重并按交易所/代码排序。

### 5.2 持久化任务、租约和结果

`NewJobQueue`、`NewRunStore`、`NewSignalSnapshotStore` 和 `NewOutbox` 实现对应 port。入队只接受零 Attempts、未取消的 PENDING Run；同 kind 与幂等键返回原 Run，输入 hash 不同则拒绝。所有身份按字节精确匹配。

领取使用单条有序 `UPDATE ... ORDER BY created_at, id LIMIT 1`，当前不依赖 `SKIP LOCKED`。每次领取生成独立的 256 位随机 token，数据库 `UTC_TIMESTAMP(6)` 计算租约期限和重试就绪状态。Run.Attempts 返回持久化领取次数，最多4次（初次加3次重试）；取消任务不能领取。续租、重试和发布先锁定 run/token 记录，再用 run/owner/token、未过期和未取消条件更新，旧 worker 无法覆盖重领结果。

`Retry` 清理租约并写入下次执行时间；第四次执行或不可重试错误直接进入 FAILED。应用 worker 必须通过 `port.JobQueue.ReapExpired(ctx)` 周期清理已耗尽次数的过期租约，每轮最多1000个。取消请求在行锁下直接进入 CANCELLED，同时保留数据库 UTC 请求时间；计算中的 worker 应通过 Get 检查并退出。

扫描快照、回测汇总、订单、成交、权益点、完成 outbox 及 Run 终态在同一事务提交。明细单批最多1000行，最终更新再次检查租约期限；任一批次、outbox 或最终 CAS 失败全部回滚。扫描带失败项时保存 PARTIAL_SUCCEEDED。Latest 只读取已发布成功/部分成功快照，快照行按证券稳定排序，结果分页使用持久化 sequence。Latest 读取快照行时 JOIN `t_instruments` 取出证券当前名称（主数据中不存在的证券不出现在行内）；失败行名称按 (exchange, code) 单次批量查询解析，查询失败降级为省略并记录告警，不产生读取错误。名称是读取时点的显示属性，不随快照固化。回测 Trades 接口返回 Fill 成交明细；领域 round-trip Trade 和 FinalPosition 没有独立读取端口，已由 Summary 与持仓布尔状态提供汇总。

快照分页必须绑定精确 SnapshotID：只有 `AfterSequence=0` 且 `SnapshotKey.SnapshotID` 为空时可选最新快照；返回的 `SignalSnapshot.ID` 及 `Key.SnapshotID` 是后续页绑定值。`AfterSequence>0` 缺失 ID 直接返回 ErrInvalidPortValue，不会重新选择最新快照。提供 ID 后仍同时匹配策略 ID、策略版本、参数 hash，以及非零 AsOf；未知、键不匹配或未发布的快照返回 ErrSnapshotNotReady。ID 的大小写和尾空格均精确区分。同 AsOf 后续发布更高 DataVersion 也不会改变已经开始的分页。API 必须把 `snapshot_id` 与 `after_sequence` 一起传入；不能仅以 AsOf/DataVersion 代替快照身份。

同业务条件、DataVersion 和 AsOf 可以由不同 Run 重扫并发布不同 Snapshot；`uq_snapshot_id` 和 `uq_snapshot_run` 仍保证快照身份与每 Run 一个结果。Migrate 在确认保留索引已建立后，显式查询并删除旧 `uq_snapshot_business` 唯一索引；此升级不依赖 AutoMigrate 删除旧索引，不删除数据，重复迁移安全。查询复用现有 `idx_snapshot_latest` 的策略/参数/状态前缀，并按 AsOf、DataVersion、自增 ID 选择最新发布。旧 SnapshotID 的分页始终不变。

`JobQueue.FindByIdempotency` 提供精确 kind/key 只读查询，使应用在最新行情变化后仍可返回原任务；Enqueue 的事务唯一约束继续负责并发仲裁。`MarketDataRepository.MarketChanges` 在 DirtyInstruments 之外增加一次有界 EXISTS 查询，对 `(after, through]` 的因子或公司行动修订作全快照失效标记；纯 Bar 修订保留 dirty 增量路径。所有过滤绑定确切 COMPLETE 版本，不做逐证券因子比较。

Enqueue 的同幂等键异 hash 冲突以 `port.ErrIdempotencyConflict` 明确返回，并兼容 ErrInvalidPortValue；应用才能把并发系统输入解析漂移与真正的用户请求冲突区分开。碰撞事务回滚后由应用读取胜出者并验证完整保存输入，数据库错误或身份校验错误不伪装为幂等碰撞。

回测证券优先核对请求和每条非空 Order/Fill Instrument；请求只解析稳定的 instrument、parameters、config 字段，不导入应用 DTO。无请求证券时可由领域结果补全；没有任何合法证券或证券不一致时拒绝写入。订单/成交标识保持 LONGBLOB 字节完整。

Outbox 的 Payload 是端口定义的不透明字节，用 JSON base64 字符串无损保存；消费者需先 JSON 解码为字节，再按事件协议解码。独立 Publish 按 EventID 幂等，重复 ID 的内容不同会拒绝。成功/部分成功/Fail 发布生成稳定的 compute.completed 事件 ID。当前没有外部投递器，PublishedAt 保持空值供后续消费。

事务只重试已经回滚的 MySQL 1213/1205 锁冲突，最多3次、退避遵守 context；commit 返回错误时不重放、不报告成功或有效租约。真实并发、过期重领、取消、结果分批回滚和提交可见性必须通过 MySQL 8.4 集成门禁。

### 5.3 自选清单与报价查询

`t_watchlist` 是单用户自选表：`BaseModel` + `Exchange`+`Code` 组合唯一索引 `uq_watchlist`，无用户列；排序即插入顺序（id 升序），上限 100 由应用层校验。`WatchlistStore` 实现对应 port：`List` JOIN `t_instruments`（exchange+code 相等且 active）返回完整身份与行 ID，非活跃条目不返回但保留在表中；`Add` 使用 `clause.OnConflict{DoNothing}` 幂等写入；`Remove` 对不存在条目也成功；`Count` 供上限检查。

`LatestDailyQuotes` 用单条窗口函数 SQL 批量读取：每只证券取最新两根当前日线 bar（`timeframe='DAY' AND valid_to_version IS NULL`，`ROW_NUMBER() OVER (PARTITION BY instrument_id ORDER BY close_time DESC)`，rn≤2），价格按 `ValueScale`（10000）换算为元，`change=close-prev_close`、`change_pct=change/prev_close*100`；不足两根 bar 时相应字段为 null。空集合跳过查询，不按证券循环。

### 5.4 行情看板

`t_chart_boards` 是单用户全局看板表：`BaseModel` + `Name`（VARCHAR(40)）+ `Config`（JSON 文本，应用层校验并规范化后写入、读取原样透传）+ `is_active` 布尔；无用户列，行数上限 20 由应用层校验，无额外索引。`ChartBoardStore` 实现对应 port，每个变更在单事务内维护恰一激活不变量：创建即激活并清除其余；更新先 Take 确认存在再无条件 Updates——MySQL 默认连接不含 CLIENT_FOUND_ROWS，同值 UPDATE 的 RowsAffected 为 0，不能以受影响行数判定 NotFound；激活先设目标再清其余；删除激活看板后按 id 升序取剩余首块接替。删除激活的末板会短暂产生零激活状态，该路径由应用层末板保护前置拒绝，存储不重复裁决。

### 5.5 兼容查询

HTTP 兼容查询使用两个只读扩展：`MarketDataRepository.ResolveCode` 验证六位数字后只读 active 证券，最多返回两个匹配，由 API 区分无匹配/唯一/歧义；不推断交易所。`SignalSnapshotStore.LatestPublishedKey` 只读 SUCCEEDED/PARTIAL_SUCCEEDED，按 AsOf、DataVersion、自增 ID 倒序定位，返回完整 SnapshotID/版本/参数 hash。版本可省略供旧接口跨版本选最新，非空 SnapshotID 精确匹配。后续行读取仍使用严格 SnapshotKey.Validate 与 SnapshotID 绑定，不放宽原 Latest 的分页约束。

### 5.6 旧行情迁移命令

先在备份副本演练，正式运行时暂停旧行情采集和扫描：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size 1000
```

默认 dry-run 只读取旧表并完成转换校验，不建表、不写数据库。apply 仅用于初始化版本库：开始写暂存前拒绝既有业务版本或孤立市场修订；只允许版本0分配锁，或本迁移尚未完成的真实版本1。已有证券元数据可以保留。成功迁移再次运行返回相同版本和报告。

apply 在固定专用连接上持有 MySQL GET_LOCK，并在所有退出路径尝试 RELEASE_LOCK；取锁结果不确定时也尝试释放。无法确认释放时通过 driver.ErrBadConn 丢弃物理连接，防止归还连接池后仍遗留命名锁。连接池须允许至少2条连接（命令固定2条；上限1在取锁前拒绝）：专用连接以只读 Repeatable Read 事务读取一致源快照，第二条连接逐批提交暂存和检查点。进程退出不会丢失已提交批次。

`t_legacy_kernel_migration` 使用 `(stage, instrument_key, legacy_id)` 复合主键和 UTC 微秒审计字段；独立的源主键索引和证券索引服务两类分页。旧表按主键读取，每批立即校验、增量计算 SHA-256/统计并持久化源行及游标，然后才读取下一批。恢复时重新流式验证已复制前缀，已有行不重写；源扫描完成后冻结摘要，任何新增、删除或修改均拒绝续跑，并且不会因检测漂移而写入新暂存行。需要恢复原维护窗口快照，不提供破坏性自动重置。

证券只按 SSE `600/601/603/605/688/689`、SZSE `000/001/002/003/300/301`、BSE `4/8/302/920` 前缀解析六位代码。报告包含合法源证券数、日周 Bar 数和日期范围、最后已提交源主键（dry-run 为已读取主键）、源 SHA-256 和各证券转换批次摘要累积的发布摘要。未知代码进入拒绝清单。

转换逐证券从暂存读取历史。旧表行在本进程内直接转换为内核 Bar：价格按 DECIMAL 值缩放 10000 转 `market.Price`，非有限或超范围值拒绝；交易日期解析为 UTC 全日会话（OpenTime=CloseTime），`Trading=Tradable`，成交量原样保留。每个 timeframe 的转换结果经 `market.NewDataset` 校验（升序、无重复、OHLC 与成交量合法）。复权因子与公司行动为空列表，激活后由首次全市场刷新（新浪 `qfq.js`）补齐 QFQ 因子。通过验证的转换按证券缓存，重启无需重复转换。

目标市场行按 batch-size 分批写入真实 INCOMPLETE 版本，每批与目标游标在同一短事务提交。目标证券全部写完后按批读取并校验完整摘要，写入验证凭证。中途失败只重做未提交批次；已验证证券不重复写入。最后一次轻量事务只检查凭证数量并更新版本和报告为 COMPLETE，不搬运市场历史。普通 Publish 遇到本迁移的 INCOMPLETE 版本会拒绝新发布，防止更高版本间接暴露部分数据；版本查询始终拒绝 INCOMPLETE。

每个外部目标批次内部再按模型可绑定列数拆分 INSERT，单条最多使用 64511 个参数，低于 MySQL 8.4 的 65535 参数上限并预留1024。当前 Bar/因子/公司行动分别绑定18/9/11列，对应最多3583/7167/5864行；新增可写字段会自动缩小内部块。所有内部 INSERT 和最后的目标检查点仍属于同一外部事务，任一内部块失败整批回滚；命令的 batch-size 上限仍为10000。

未知代码、无历史 Bar、日期解析失败或转换校验失败都会阻止最终切换。Failures 保留证券及稳定白名单分类：SOURCE_UNAVAILABLE、INCOMPLETE_DATA、INVALID_DATA、STORAGE_FAILURE、CANCELED，不保存底层错误、URL或凭据。任何最终事务/commit/解锁失败，返回报告均为 INCOMPLETE 且 BacktestEnabled=false；只有确认最终提交成功才启用。命令返回脱敏错误和非零退出码。未知上市状态和交易单位仍保留未激活/零值，后续主数据同步补齐。

即使遇到存储或其他迁移错误，命令也先输出已有安全 JSON 报告（含版本、检查点及证券级白名单失败分类），再返回通用错误和非零退出码；不会输出底层数据库错误或 DSN。输出通道本身失败时仅返回通用报告输出错误。

驻留内存为当前源批次、当前证券历史/转换/校验数据和证券级元数据；SQL 读写批次为1–10000。单证券历史大小仍影响峰值，正式运行需备份副本演练内存与暂存空间。检查点格式v2针对新初始化迁移；不自动接管旧原型格式或已有业务版本。

## 6. 失败、取消和一致性语义

- 事务只重试已经完整回滚的 MySQL 1213/1205 锁冲突；commit 结果不确定时不自动重放，也不报告成功。
- context 取消中止等待、重试和数据库调用；租约过期、取消或 token 不匹配统一阻止结果发布。
- 行情发布、任务终态与对应结果、扫描快照及 Outbox 分别在要求的原子事务内提交，任一步失败整体回滚。
- 迁移失败保留已提交检查点但不暴露 INCOMPLETE 版本；恢复必须重新验证已复制前缀和冻结源摘要。

## 7. 性能与安全约束

- 批量证券上限 5000，明细写入按最多 1000 行分批；SQL 参数数量从 MySQL 65535 上限扣除安全余量后计算，禁止无界 `IN`、`UNION ALL` 或结果集。
- 身份字段使用二进制精确比较，所有 SQL 参数化并显式列名；事务不包含外部 HTTP、长计算或无界循环。
- 错误与迁移报告不得泄露 DSN、SQL、路径、凭据、来源 URL 或原始响应；数据库时间统一 UTC 微秒。
- MySQL 8.4 的 DDL、索引、事务隔离和并发语义必须在真实实例验证。

## 8. 测试与验收证据

```bash
go test ./internal/infrastructure/mysql -cover
go test -tags=integration ./internal/infrastructure/mysql/... -run '^$'
go test -race -tags=integration ./internal/infrastructure/mysql -run '^TestDurable' -count=1
go test -tags=integration ./internal/infrastructure/mysql -run TestLegacyMigrationMySQLRestartAndIncompleteVisibility
go test -tags=integration ./internal/infrastructure/mysql/... -count=1
```

单元与 SQL mock 测试验证端口校验、SQL 边界、映射和错误传播；真实集成测试读取仓库根目录下、本地保存且不纳入 Git 的 `config.yaml`，连接获批远端 MySQL 8.4.x/x86_64 服务，并为每个测试创建随机隔离数据库，覆盖迁移、索引、锁、租约、提交可见性、失败恢复和幂等重跑，结束后删除。夹具必须先只读验证版本与编译架构，且绝不使用配置中的业务库；创建结果不确定时也必须尝试幂等删除随机数据库。缺少配置、配置无效或清理失败时明确失败，不能以本地 MySQL、mock 或只编译替代。

## 9. 相关文档

- [系统设计](../../../architecture/system-design.md)
- [领域地图](../../README.md)
- [应用端口设计](../port.md)
- [应用层设计](../application.md)
- [行情领域设计](../market.md)
- [旧行情迁移命令设计](../../cmd/migrate-strategy-kernel.md)

## 双服务共享库

两个角色共用同一 NAS 业务库，版本、证券关联、队列、看板与结果索引不变。updater 通过 data.New 完成启动迁移并独占行情发布；workbench 通过 data.Open 只连接，读取已发布行情并写入工作台和计算表。本次不新增表/列/索引，也不拆分数据库；上线先初始化 updater，再启动 workbench。DDL 归属变化由 `data/service_integration_test.go` 在远端随机隔离库验证，禁止在 NAS 业务库执行测试。

### 更新进度观察存储

新增 `t_market_refresh_runs` 与 `t_market_refresh_failures`，Migrate 负责初始化与索引检查。任务 run_id 为 VARBINARY(64) 唯一身份，类型与来源为有限枚举；时间 UTC DATETIME(6)。摘要列包括 kind、trigger_source、state、total、succeeded、failed、started_at、finished_at、heartbeat_at、last_progress_at、snapshot_at、revision、error_code。索引按(kind,id)历史、(kind,started_at,id)最新、(state,id)重启恢复组织。

失败项以(run_id,exchange,code)唯一，按(run_id,id)分页，显示名称 LEFT JOIN 证券主数据，不为成功证券复制明细。SaveRefresh 在单个短事务内插入或锁定任务、拒绝过期 revision/终态回写，以绝对计数更新摘要并按唯一身份写入失败明细；无外部采集调用。重启恢复只匹配固定启动时刻前的非终态。查询固定列、有限分页（最大100）。历史不自动删除，进度表不参与行情筛选、队列或断点恢复。

更新进度的最新任务/指定任务不存在时仍返回 `ErrRefreshRunNotFound`，由应用层映射为空状态或 HTTP 404；这是预期查询结果，不输出 GORM `record not found` 错误日志。真实查询错误仍传播。
