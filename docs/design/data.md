---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["data/", "model/"]
related: []
---

# 财报与兼容数据访问设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `data`、`model` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

`data` 提供财报、证券主数据和旧技术 K 线的 GORM Repository；`model` 保存对应兼容模型。正常生产运行只使用财报和证券主数据，旧日/周 K 线 Repository 与 Model 仅供历史迁移或回滚工具读取。

本模块不保存新版本化行情、不实现任务/快照存储、不承载业务规则。新内核持久化统一位于 `internal/infrastructure/mysql`。

## 2. 对外能力与使用者

- `Data.New` 建立 MySQL 连接池，迁移运行时表和新内核表，成功后转移连接所有权。
- `FinancialReportRepo` 批量 upsert、按代码查询和列出已有代码。
- `StockInfoRepo` 读取和批量保存证券主数据。
- `StockKlineDailyRepo`、`StockKlineWeeklyRepo` 提供迁移/回滚所需的旧表读取与兼容 CRUD，但生产服务不得调用。

`business` 使用财报和证券 Repository；迁移命令使用旧 K 线读取边界。

## 3. 依赖和边界

本模块是允许访问 GORM 的两个位置之一，另一个是新内核 MySQL adapter。业务和 API 不得自己拼写 SQL。

正常启动的 `runtimeModels` 只包含 `FinancialReport` 和 `StockInfo`，随后调用新内核 `Migrate`。旧 `StockKlineDaily`/`StockKlineWeekly` 不参与 AutoMigrate，避免启动时创建或改变旧技术表。

## 4. 模型与持久化不变量

- 新增模型显式实现 `TableName()`，表名使用 `t_` 前缀和 snake_case；一张表一个文件。
- UTC 瞬时字段使用 `time.Time` / `DATETIME(6)`，持久化前规范化到 UTC 微秒。
- 只有纯交易日使用 date-only 语义，不新增自由格式日期字符串。
- 金额和价格使用项目定点整数类型；外部财报兼容字段是明确例外。
- 可变业务记录显式保留创建/更新时间；版本化行情用有效版本区间而不是软删除模拟历史。
- 索引、唯一性、精度和可见区间必须显式声明，不能依赖 ORM 默认值。
- 旧 K 线日期字符串只供迁移读取，不得作为新模型范例。

## 5. 初始化与查询流程

```text
解析配置
  → 打开 GORM / sql.DB
  → 配置连接池与 UTC DSN
  → 迁移财报、证券主数据和新内核
  → 成功后把连接交给 Data
```

任一初始化或迁移步骤失败都会关闭连接；只有完整成功才由调用者负责在进程停机时关闭。

Repository 方法接受 context，查询具有 limit/offset 或明确代码范围，批量写入由调用者控制输入规模。

## 6. 失败和一致性语义

- 数据库错误必须带操作上下文返回，禁止静默忽略。
- `record not found` 只有业务契约允许空结果时才转换为空；其他情况保留错误。
- 批量 upsert 依赖数据库唯一约束仲裁，不能只用应用层预查保证一致性。
- Repository 不在事务中发起外部 HTTP 或运行长计算。

## 7. 性能与安全约束

- DSN 使用 `parseTime=True&loc=UTC`；不得记录包含用户名、密码和地址的完整 DSN。
- 连接池默认最大打开 60、空闲 10、生命周期 30 分钟，可由配置覆盖。
- 查询显式列出稳定排序和边界；复杂新内核批量查询不得回流到本兼容模块。

## 8. 测试与验收证据

测试覆盖运行时模型排除旧 K 线、UTC DSN、初始化失败关闭连接、成功转移所有权，以及财报和 Repository 错误传播。

```bash
go test ./data -cover
```

## 9. 相关文档

- [系统设计](../architecture/system-design.md)
- [财报业务设计](business.md)
- [MySQL 新内核设计](internal/infrastructure/mysql.md)
- [迁移命令设计](cmd/migrate-strategy-kernel.md)
