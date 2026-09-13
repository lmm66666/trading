# 策略内核 MySQL 存储

本目录提供 MySQL 5.7/8.0 的版本化行情仓储和内核表结构。一张表一个模型，领域及 port 不依赖 GORM。连接必须使用 `parseTime=true&loc=UTC`；时间以 UTC `DATETIME(6)` 存储，Price/Money 使用有符号 BIGINT，版本和序号使用无符号整数。

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
