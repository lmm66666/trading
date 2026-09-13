# 策略内核 MySQL 存储

本目录提供 MySQL 5.7/8.0 的版本化行情仓储和内核表结构。一张表一个模型，领域及 port 不依赖 GORM。连接必须使用 `parseTime=true&loc=UTC`；时间以 UTC `DATETIME(6)` 存储，Price/Money 使用有符号 BIGINT，版本和序号使用无符号整数。

不透明身份字段采用 VARBINARY，保证 `Key`、`key`、`key ` 精确区分，避免 `_ci` 排序规则及 PAD SPACE 的隐式合并。RunID、SnapshotID、EventID、StrategyID、摘要/参数 hash 为 64 字节；策略/引擎版本为 32 字节；幂等键、租约 owner/token、AggregateID、Source、SourceEventID 为 128 字节。`port.ValidateIdentity` 及各 DTO Validate 校验相同的 UTF-8 字节上限，不 trim 或修改身份；接受标量身份参数的后续适配器也应调用该验证。普通状态、类型、名称、原因保持文本。当前 OrderID 把无长度限制的 Reason 拼入标识，OrderID/FillID 因而使用不参与索引的 LONGBLOB，保持字节完整；以后可单独将领域 ID 改成固定长度标识，本迁移不改领域语义。

连接初始化在所有迁移完成前持有所有权，旧表或内核迁移任一步失败都释放连接，成功后才交给 Data。若已有早期测试数据，切换身份列前需确认其字节长度符合端口边界，不能依赖数据库截断修复超长旧值。

`Migrate` 按依赖顺序创建 13 张内核表，检查关键索引，并幂等建立版本 0 锁行。版本 0 的状态为 `INTERNAL_LOCK`、来源为 `__kernel_version_lock__`，只用于序列化发布，永远不能作为已完成数据版本读取。旧表继续由 `data.New` 注册迁移，直到 Task15 切换运行时。

`Publish` 是增量 upsert：未提供的 Bar、因子或事件表示本次未观测，不表示删除；当前端口没有权威快照或删除语义。批次先复制、排序、校验并计算 SHA-256；提供的 Digest 必须匹配 `MarketBatchDigest`。输入版本号不参与摘要，发布版本由仓储分配。仅该证券最近一条 COMPLETE 发布可命中摘要幂等，重新提交较老内容会形成新版本。

发布事务锁定内部锁行，按 `(exchange, code)` 解析证券，写入 PENDING 版本，只关闭发生变化的当前修订，批量插入替代修订，最后标记 COMPLETE。首次遇到证券时，未知 Name/Board 保留空串，Active 为 false、LotSize 为 0；后续主数据同步负责补齐并激活。事件 ID 在单个证券内必须由上游保证来源唯一，使用 VARBINARY 精确保存大小写及尾空格，修订键包含版本以保留历史。

COMPLETE 表示已提供变更全部提交。当前写入端口没有上游质量字段，因此不会根据“没有公司行动/没有因子”推断真实数据缺失。数据版本 Quality 记录 COMPLETE；需要复权数据的消费者必须检查对应时段因子是否可用，不能把该质量标志当成收益可信证明。

请求版本必须已完成且大于 0。历史可见区间为 `[valid_from_version, valid_to_version)`；批量读取在只读 Repeatable Read 事务内完成，最新版本可以安全使用 current 索引条件。Dataset 按请求版本重新标记和校验；单个证券的非法数据只产生该证券错误，数据库读取失败则显式返回所有受影响证券的错误。

`BatchDatasets` 最多接受 5000 个证券，基础读取固定 6 条 SELECT：请求版本、最新版本、证券、所有周期 Bar、因子、公司行动。`LookbackBars` 表示 From 前最多 N 根 Bar；通过有 LIMIT 的参数化 UNION ALL 读取，最多再加 2 条 SELECT。每条最多 7500 个分支、45000 个参数，低于 MySQL 65535 参数上限。不会将所有历史 Bar 搬到应用层裁剪；长 UNION 的解析成本和查询包大小仍需真实环境压测。调用者应控制窗口、标的数及暖机根数，避免请求本身产生过大结果集。

`DirtyInstruments(after, through)` 合并 Bar、因子、公司行动在 `(after, through]` 内的新修订，使用三组 `(valid_from_version, instrument_id)` 索引，去重并按交易所/代码排序。

验证命令：

```bash
go test ./internal/infrastructure/mysql -cover
go test -tags=integration ./internal/infrastructure/mysql/... -run '^$'
go test -tags=integration ./internal/infrastructure/mysql/... -count=1
```

集成测试使用 testcontainers MySQL 模块 v0.44.0。无 Docker 会明确失败，不跳过或伪报通过；SQL mock 测试仅验证 SQL 边界、映射和错误传播，不能替代真实 DDL、事务隔离、并发锁及索引验收。

## 持久化任务、租约和结果

`NewJobQueue`、`NewRunStore`、`NewSignalSnapshotStore` 和 `NewOutbox` 实现对应 port。入队只接受零 Attempts、未取消的 PENDING Run；同 kind 与幂等键返回原 Run，输入 hash 不同则拒绝。所有身份按字节精确匹配。

领取使用 MySQL 5.7 兼容的单条有序 `UPDATE ... ORDER BY created_at, id LIMIT 1`，不依赖 SKIP LOCKED。每次领取生成独立的 256 位随机 token，数据库 `UTC_TIMESTAMP(6)` 计算租约期限和重试就绪状态。Run.Attempts 返回持久化领取次数，最多4次（初次加3次重试）；取消任务不能领取。续租、重试和发布先锁定 run/token 记录，再用 run/owner/token、未过期和未取消条件更新，旧 worker 无法覆盖重领结果。

`Retry` 清理租约并写入下次执行时间；第四次执行或不可重试错误直接进入 FAILED。应用 worker 必须通过 `port.JobQueue.ReapExpired(ctx)` 周期清理已耗尽次数的过期租约，每轮最多1000个。取消请求在行锁下直接进入 CANCELLED，同时保留数据库 UTC 请求时间；计算中的 worker 应通过 Get 检查并退出。

扫描快照、回测汇总、订单、成交、权益点、完成 outbox 及 Run 终态在同一事务提交。明细单批最多1000行，最终更新再次检查租约期限；任一批次、outbox 或最终 CAS 失败全部回滚。扫描带失败项时保存 PARTIAL_SUCCEEDED。Latest 只读取已发布成功/部分成功快照，快照行按证券稳定排序，结果分页使用持久化 sequence。回测 Trades 接口返回 Fill 成交明细；领域 round-trip Trade 和 FinalPosition 没有独立读取端口，已由 Summary 与持仓布尔状态提供汇总。

回测证券优先核对请求和每条非空 Order/Fill Instrument；请求只解析稳定的 instrument、parameters、config 字段，不导入应用 DTO。无请求证券时可由领域结果补全；没有任何合法证券或证券不一致时拒绝写入。订单/成交标识保持 LONGBLOB 字节完整。

Outbox 的 Payload 是端口定义的不透明字节，用 JSON base64 字符串无损保存；消费者需先 JSON 解码为字节，再按事件协议解码。独立 Publish 按 EventID 幂等，重复 ID 的内容不同会拒绝。成功/部分成功/Fail 发布生成稳定的 compute.completed 事件 ID。当前没有外部投递器，PublishedAt 保持空值供后续消费。

事务只重试已经回滚的 MySQL 1213/1205 锁冲突，最多3次、退避遵守 context；commit 返回错误时不重放、不报告成功或有效租约。真实并发、过期重领、取消、结果分批回滚和提交可见性必须通过 MySQL5.7/8.0 集成门禁：

```bash
go test -race -tags=integration ./internal/infrastructure/mysql -run '^TestDurable' -count=1
```

## 旧行情迁移命令

先在备份副本演练，正式运行时暂停旧行情采集和扫描：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size 1000
```

默认 dry-run 只读取旧表并请求来源，不建表、不写数据库。apply 仅用于初始化版本库：开始写暂存前拒绝既有业务版本或孤立市场修订；只允许版本0分配锁，或本迁移尚未完成的真实版本1。已有证券元数据可以保留。成功迁移再次运行返回相同版本和报告。

apply 在固定专用连接上持有 MySQL GET_LOCK，并在所有退出路径尝试 RELEASE_LOCK；取锁结果不确定时也尝试释放。无法确认释放时通过 driver.ErrBadConn 丢弃物理连接，防止归还连接池后仍遗留命名锁。连接池须允许至少2条连接（命令固定2条；上限1在取锁前拒绝）：专用连接以只读 Repeatable Read 事务读取一致源快照，第二条连接逐批提交暂存和检查点。进程退出不会丢失已提交批次。

`t_legacy_kernel_migration` 使用 `(stage, instrument_key, legacy_id)` 复合主键和 UTC 微秒审计字段；独立的源主键索引和证券索引服务两类分页。旧表按主键读取，每批立即校验、增量计算 SHA-256/统计并持久化源行及游标，然后才读取下一批。恢复时重新流式验证已复制前缀，已有行不重写；源扫描完成后冻结摘要，任何新增、删除或修改均拒绝续跑，并且不会因检测漂移而写入新暂存行。需要恢复原维护窗口快照，不提供破坏性自动重置。

证券只按 SSE `600/601/603/605/688/689`、SZSE `000/001/002/003/300/301`、BSE `4/8/920` 前缀解析六位代码。报告包含合法源证券数、日周 Bar 数和日期范围、最后已提交源主键（dry-run 为已读取主键）、源 SHA-256 和绑定回填结果的摘要。未知代码进入拒绝清单。

回填逐证券从暂存读取历史。旧 OHLC 复权来源不可靠，因此最终 raw OHLCV/Amount 由 Task9 provider 回填；日周线日期必须与旧数据精确匹配。合并后的因子只规范化/排序一次，再用顺序游标验证每根 Bar 的覆盖及跨周期一致性，避免逐 Bar 复制或排序。公司行动请求必须成功，合法空列表允许通过。通过验证的回填按证券缓存，重启无需再次请求已缓存证券。

目标市场行按 batch-size 分批写入真实 INCOMPLETE 版本，每批与目标游标在同一短事务提交。目标证券全部写完后按批读取并校验完整摘要，写入验证凭证。中途失败只重做未提交批次；已验证证券不重复写入。最后一次轻量事务只检查凭证数量并更新版本和报告为 COMPLETE，不搬运市场历史。普通 Publish 遇到本迁移的 INCOMPLETE 版本会拒绝新发布，防止更高版本间接暴露部分数据；版本查询始终拒绝 INCOMPLETE。

每个外部目标批次内部再按模型可绑定列数拆分 INSERT，单条最多使用 64511 个参数，低于 MySQL 5.7/8.0 的 65535 参数上限并预留1024。当前 Bar/因子/公司行动分别绑定18/9/11列，对应最多3583/7167/5864行；新增可写字段会自动缩小内部块。所有内部 INSERT 和最后的目标检查点仍属于同一外部事务，任一内部块失败整批回滚；命令的 batch-size 上限仍为10000。

未知代码、无历史 Bar、缺失日期、复权冲突或来源失败都会阻止最终切换。Failures 保留证券及稳定白名单分类：SOURCE_UNAVAILABLE、INCOMPLETE_DATA、INVALID_DATA、STORAGE_FAILURE、CANCELED，不保存底层错误、URL或凭据。任何最终事务/commit/解锁失败，返回报告均为 INCOMPLETE 且 BacktestEnabled=false；只有确认最终提交成功才启用。命令返回脱敏错误和非零退出码。未知上市状态和交易单位仍保留未激活/零值，后续主数据同步补齐。

即使遇到存储或其他迁移错误，命令也先输出已有安全 JSON 报告（含版本、检查点及证券级白名单失败分类），再返回通用错误和非零退出码；不会输出底层数据库错误或 DSN。输出通道本身失败时仅返回通用报告输出错误。

驻留内存为当前源批次、当前证券历史/回填/校验数据和证券级元数据，不保留全市场源 JSON、Kline 或 raw 批次；SQL 读写批次为1–10000。单证券历史大小仍影响峰值，正式运行需备份副本演练内存、暂存空间与来源耗时。检查点格式v2针对新初始化迁移；不自动接管旧原型格式或已有业务版本。

真实验证命令：

```bash
go test -tags=integration ./internal/infrastructure/mysql -run TestLegacyMigrationMySQLRestartAndIncompleteVisibility
```

覆盖 MySQL 5.7/8.0 的失败回填、分批目标中断、已提交游标、版本不可见性、恢复和幂等重跑；Docker缺失仍明确失败。
