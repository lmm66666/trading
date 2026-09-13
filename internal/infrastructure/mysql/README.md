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

## 旧行情迁移命令

先在备份副本演练；正式迁移须暂停旧行情采集和扫描，保持维护窗口内的源数据不变：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size 1000
```

默认 dry-run 只读取旧表并请求行情来源，不创建或写入任何数据库表。显式 apply 创建内核表和 `t_legacy_kernel_migration` 暂存表。该表具有 UTC 微秒审计字段，`(stage, legacy_id)` 主键代表已提交检查点；批次按旧表主键顺序复制，重启会校验已有内容并跳过已提交写入，已成功回填的证券复用缓存结果。manifest 的 SHA-256 绑定整个源快照，源行新增、删除或更改都会拒绝续跑，需要恢复维护窗口快照后继续；不提供破坏性自动重置。

仅接受六位股票代码，按 SSE `600/601/603/605/688/689`、SZSE `000/001/002/003/300/301`、BSE `4/8/920` 前缀映射。未知代码进入拒绝清单。报告的证券数、日周 Bar 数及日期范围指通过代码和旧值校验的源数据，`LastLegacyIDs` 包含各旧表最后读取主键。`SourceDigest` 是按表顺序、主键顺序编码源记录的 SHA-256；`Digest` 再绑定已验证回填批次的摘要，不含运行时间或分配的版本号。

旧 OHLC 的复权口径无法可靠追溯，因此它们仅作为日期覆盖与源校验依据，最终原始 OHLCV/Amount 来自 Task9 provider 的 raw 回填。必须逐日期匹配旧日线和周线，并验证 QFQ 因子覆盖、跨周期因子一致性以及公司行动请求成功。合法的空公司行动列表可以接受；未知代码、无历史 Bar 的证券、缺失日期、复权冲突或来源失败均产生 INCOMPLETE。INCOMPLETE 持久化到版本表和报告检查点，不写入可见市场修订，版本查询会拒绝该版本。

全部证券通过后，在同一个版本发布事务内写入市场数据并标记 COMPLETE，避免部分标的提前可见；成功后再运行返回相同版本和报告。未知上市状态和交易单位保留未激活/零值，后续主数据同步负责补齐。命令错误输出脱敏，不打印 DSN、配置内容和数据库错误；非完整结果返回非零退出码。

当前为离线迁移实现：SQL 读写有批次限制（1–10000），但源记录、分组 Bar 和回填批次仍驻留内存，最终发布使用单事务。全市场运行前需在备份副本验证内存、暂存空间和事务容量；该限制不能用小批次参数消除。真实迁移集成测试为 `go test -tags=integration ./internal/infrastructure/mysql -run TestLegacyMigrationMySQLRestartAndIncompleteVisibility`，覆盖 MySQL 5.7/8.0 的失败回填、版本不可见性、恢复和幂等重跑。
