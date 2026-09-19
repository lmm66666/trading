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

# 兼容数据访问设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `data`、`model` |
| 最后更新 | 2026-09-19 |

## 1. 职责与非职责

`data` 建立 MySQL 连接池并执行启动迁移；`model` 仅保存旧库迁移链路仍依赖的兼容模型（`StockInfo` 与旧 K 线读取模型）。迁移命令 `cmd/migrate-strategy-kernel` 通过这些模型读取旧表；正常生产服务不读取旧 K 线。

本模块不保存新版本化行情、不实现任务/快照存储、不承载业务规则。新内核持久化统一位于 `internal/infrastructure/mysql`。`data` 包整体收敛进组合根属后续变更。

## 2. 对外能力与使用者

- `Data.New` 建立 MySQL 连接池，迁移运行时表和新内核表，成功后转移连接所有权。
- `runtimeModels` 仅包含 `StockInfo`（`t_stock_info`），服务迁移器 stage "info" 的证券名称读取。

使用者：组合根（连接与内核装配）、迁移命令（旧表读取边界）。

## 3. 依赖和边界

本模块是允许访问 GORM 的两个位置之一，另一个是新内核 MySQL adapter。业务和 API 不得自己拼写 SQL。

正常启动的 `runtimeModels` 只包含 `StockInfo`，随后调用新内核 `Migrate`。旧 `StockKlineDaily`/`StockKlineWeekly` 不参与 AutoMigrate，避免启动时创建或改变旧技术表；已删除的 `FinancialReport` 不再重建 `financial_reports` 表。

## 4. 模型与持久化不变量

- 新增模型显式实现 `TableName()`，表名使用 `t_` 前缀和 snake_case；一张表一个文件。
- UTC 瞬时字段使用 `time.Time` / `DATETIME(6)`，持久化前规范化到 UTC 微秒。
- 只有纯交易日使用 date-only 语义，不新增自由格式日期字符串。
- 旧 K 线日期字符串只供迁移读取，不得作为新模型范例。
- 索引、唯一性、精度和可见区间必须显式声明，不能依赖 ORM 默认值。

## 5. 初始化流程

```text
解析配置
  → 打开 GORM / sql.DB
  → 配置连接池与 UTC DSN
  → 迁移 t_stock_info 和新内核
  → 成功后把连接交给 Data
```

任一初始化或迁移步骤失败都会关闭连接；只有完整成功才由调用者负责在进程停机时关闭。

## 6. 失败和一致性语义

- 数据库错误必须带操作上下文返回，禁止静默忽略。
- Repository 不在事务中发起外部 HTTP 或运行长计算。

## 7. 性能与安全约束

- DSN 使用 `parseTime=True&loc=UTC`；不得记录包含用户名、密码和地址的完整 DSN。
- 连接池默认最大打开 60、空闲 10、生命周期 30 分钟，可由配置覆盖。

## 8. 测试与验收证据

测试覆盖运行时模型仅保留迁移主数据表、UTC DSN、初始化失败关闭连接和成功转移所有权。

```bash
go test ./data -cover
```

## 9. 相关文档

- [系统设计](../architecture/system-design.md)
- [MySQL 新内核设计](internal/infrastructure/mysql.md)
- [迁移命令设计](cmd/migrate-strategy-kernel.md)
