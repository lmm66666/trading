# 旧行情迁移命令设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `cmd/migrate-strategy-kernel` 与 MySQL LegacyMigrator |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

本命令在维护窗口内检查旧日/周 K 线，并将经过外部行情源回填和完整性验证的数据迁移到版本化策略内核。它支持只读 dry-run、显式 apply、持久化检查点和幂等重跑。

本命令不在正常服务启动时运行，不推断证券 active 或 lot_size，不在线双写，也不把不完整数据切换为可消费版本。

## 2. 操作入口

先在数据库备份副本执行：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true -batch-size=1000
```

确认报告无缺失后，在停止旧行情采集和扫描的维护窗口执行：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size=1000
```

`dry-run` 默认为 true；batch-size 为 1–10000。参数、配置和输出错误均使用脱敏消息。

## 3. 依赖和边界

- 命令单独打开最多 2 条 MySQL 连接，不调用 `data.New`，保证 dry-run 不执行任何 AutoMigrate。
- apply 才初始化新内核表，并调用 MySQL adapter 的 LegacyMigrator。
- 外部回填通过 Broker 获取 raw 行情、复权因子和公司行动。
- 配置文件最多读取 1MiB、启用 YAML KnownFields，DSN 使用结构化驱动配置且不进入日志。

## 4. 核心不变量

### 4.1 维护窗口与锁

- apply 使用专用连接持有 MySQL 命名锁，所有退出路径都尝试释放。
- 无法确认释放时丢弃物理连接，不能把可能持锁的连接归还连接池。
- 开始暂存前拒绝既有业务版本、孤立修订或不属于本迁移的版本 1。

### 4.2 源快照和检查点

- 源读取在专用只读 Repeatable Read 事务中形成一致快照。
- 旧表按主键分页，每批校验、更新 SHA-256/统计并持久化源行和游标后才读取下一批。
- 恢复时重新验证已经复制的前缀；源扫描完成后冻结摘要，任何新增、删除或修改都会拒绝续跑。
- 未知证券代码、无历史 Bar、日期缺失、复权冲突或来源失败阻止最终切换。

### 4.3 回填与发布

- 旧 OHLC 复权来源不可信，最终 raw OHLCV/Amount 由外部来源回填；旧日/周日期必须精确匹配。
- 因子排序后一次顺序验证覆盖；公司行动请求必须成功，明确的空列表合法。
- 目标按 batch-size 写入真实 `INCOMPLETE` 版本，每批数据与目标游标在同一短事务提交。
- 所有证券完成后重新计算目标摘要并保存验证凭证；最后一个轻量事务只在凭证完整时切换为 `COMPLETE`。
- 普通 Publish 遇到迁移中的 INCOMPLETE 版本必须拒绝，新计算始终无法读取部分迁移数据。

### 4.4 身份解析

六位股票代码只按显式前缀规则映射 SSE、SZSE、BSE；未知代码进入拒绝清单。迁移不根据旧数据推断上市状态和交易单位，迁移后必须补齐证券主数据再激活。

## 5. 失败、恢复和输出

- 暂存和目标批次都可幂等重跑；失败后只重做未提交批次，已验证证券不重复请求来源。
- 任何最终事务、commit 或解锁失败，报告必须保持 INCOMPLETE 且 BacktestEnabled=false。
- 即使存储失败，也先输出已有安全 JSON 报告，再返回通用非零错误；输出通道失败只返回通用输出错误。
- 证券级失败只保存 SOURCE_UNAVAILABLE、INCOMPLETE_DATA、INVALID_DATA、STORAGE_FAILURE、CANCELED 等白名单分类，不保存 URL、DSN 或底层正文。

## 6. 性能与资源约束

- 常驻内存只包含当前源批次、当前证券历史/回填/校验数据和证券元数据，不保留全市场原始 JSON。
- 外部 batch-size 上限 10000；内部 INSERT 根据可绑定列数自动拆分，单条最多 64511 个参数，低于 MySQL 65535 上限并预留 1024。
- 单证券历史仍决定峰值内存和来源耗时，正式执行前必须在备份副本演练。

## 7. 测试与验收证据

单元测试覆盖默认 dry-run、显式 apply、参数/配置/连接脱敏、报告输出和连接所有权。真实集成测试覆盖失败回填、批次中断、检查点恢复、版本不可见和幂等重跑。

```bash
go test ./cmd/migrate-strategy-kernel
go test -tags=integration ./internal/infrastructure/mysql -run TestLegacyMigrationMySQLRestartAndIncompleteVisibility
```

MySQL 8.4 集成测试不可由 SQL mock 替代。

## 8. 相关文档

- [系统设计](../../docs/architecture/system-design.md)
- [旧数据访问设计](../../data/DESIGN.md)
- [MySQL 设计](../../internal/infrastructure/mysql/DESIGN.md)
- [Broker 设计](../../pkg/broker/DESIGN.md)
