---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["business/", "pkg/indicator/kdj.go", "pkg/indicator/ma.go", "pkg/indicator/macd.go", "pkg/indicator/macd_test.go", "pkg/indicator/round.go", "pkg/indicator/kdj_test.go", "shell/save_financial_report.sh"]
related: []
---

# 财报与宏观业务设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `business` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

本模块保留财报采集与增量更新、财报查询和筛选信号、SHIBOR/汇率查询、证券名称读取及财报批量调度等业务。它属于早期业务栈，但仍是生产能力。

本模块不再承载技术指标、K 线信号、技术策略、扫描或回测；这些能力只能通过 `internal` 新内核实现。不得为了兼容旧 API 恢复已删除的技术策略 service。

## 2. 对外能力与使用者

- `FinancialReportService` 首次保存近 5 年、20 个季度财报，并按最近 4 期做增量补充。
- `QueryService` 按股票代码分页读取财报。
- `SignalService` 使用 `internal/financialscreen` 对已有财报执行增长筛选。
- `MacroService` 查询指定或全部期限 SHIBOR，以及指定或批量汇率。
- `FinancialScheduler` 在进程生命周期内接受手工触发，对已有股票代码有界并发更新财报。
- `StockInfoProvider` 从证券主数据提供名称，保留文件同步兼容入口。

API handler 和组合根是主要调用者。

## 3. 依赖和边界

模块可以依赖 `data` Repository、`model` 兼容模型、`pkg/broker` 外部来源和 `internal/financialscreen` 纯筛选器。它不得直接依赖 Gin 或新内核的 MySQL 具体模型。

股票代码先经 `toSymbol` 转成来源 symbol；外部数据由 Broker 解析，Repository 错误使用 `%w` 保留原因。

## 4. 核心规则

- 首次财报保存请求 20 期；空来源结果视为无数据，不写入空批次。
- 增量更新读取已存 ReportDate，只写最近 4 期中尚不存在的日期。
- 财报查询只返回财报模型，不夹带旧 K 线结果。
- 筛选错误不得被吞掉或退化为空信号。
- 宏观“全部期限/全部币种”允许部分来源失败；只有全部失败时返回整体错误，成功项仍保持稳定排序。
- 调度器必须先 Start；同时只允许一次触发，Stop 幂等并等待所有后台任务退出。
- 财报批量 Worker 当前最大并发 100，context 取消会停止未完成工作。

## 5. 主要流程

```text
API / Scheduler
  → 校验代码或宏观参数
  → Broker 获取外部数据
  → 财报去重或宏观聚合
  → Repository 持久化 / DTO 返回
```

财报筛选流程为“Repository 读取 → 构造筛选器 → 对每只证券 Match → 汇总信号”，不读取技术行情。

## 6. 失败与生命周期

- Broker、Repository 和筛选错误带操作上下文向上传播；API 负责最终脱敏。
- 调度 TriggerNow 接受后在根生命周期 context 中运行，不随触发 HTTP 请求结束而取消。
- 旧 `business` 仍使用标准库日志，后续只在相关需求中逐步迁移，不因无关变更整体重写。

## 7. 性能与安全约束

- 批量更新使用固定并发 Worker，不按证券无限创建 goroutine。
- 不记录完整财报响应、数据库配置或来源凭据。
- 新增财报查询必须有明确 limit、offset 和稳定排序，不把全表无界加载作为默认接口。

## 8. 测试与验收证据

测试覆盖财报首次/增量保存、空结果、Repository/Broker 错误、筛选搬迁、宏观部分失败、调度互斥、取消和并发上限。

```bash
go test ./business -cover
```

## 9. 相关文档

- [系统设计](../architecture/system-design.md)
- [旧数据访问设计](data.md)
- [财报筛选设计](internal/financialscreen.md)
- [Broker 设计](pkg/broker.md)
