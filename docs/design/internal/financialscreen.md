---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["internal/financialscreen/"]
related: []
---

# 财报筛选模块设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `internal/financialscreen` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

本模块提供基于季度财报的纯内存筛选规则，目前包含营业收入增长和净利润增长。它从旧业务包迁出，避免财报规则与技术策略内核混合。

本模块不读取 Repository、不抓取财报、不组合证券列表、不产生技术面信号，也不持久化筛选结果。

## 2. 对外能力与使用者

- `NewRevenueGrowth(threshold, quarterCount)` 创建收入同比增长筛选器。
- `NewProfitGrowth(threshold, quarterCount)` 创建利润同比增长筛选器。
- `ReportFilter.Match` 判断一组财报是否连续满足阈值。

`business.SignalService` 负责读取财报、创建筛选器并汇总证券结果。

## 3. 依赖和边界

只依赖 Go 标准库和 `model.FinancialReport` 兼容值。输入切片由调用者所有，筛选器复制并排序，不能改变 Repository 返回顺序。

若未来财报领域模型迁出 `model`，应在独立需求中先定义新领域值，再改变本边界。

## 4. 核心模型与不变量

- threshold 必须为有限数；quarterCount 至少为 1；取值函数不能为 nil。
- 财报先按 ReportDate 升序排列，累计披露值按同年相邻 ReportType 相减还原单季度值。
- 最近 `quarterCount` 个报告期都必须存在上年同期单季度值。
- 上年同期不能为零，当前单季度值必须为正，且每期同比增长率都不低于 threshold。
- 输入期数不足、日期年份非法、缺少上年同期或任一期不达标都返回 false。
- 不改变输入切片或财报对象。

## 5. 主要流程

```text
复制并按报告日期排序
  → 建立年度累计值
  → 还原单季度值
  → 检查最近 N 期的上年同期和增长率
  → 全部满足才 Match=true
```

## 6. 失败语义

构造参数非法返回 `ErrInvalidFilter`。`Match` 对数据缺失或不符合条件返回 false，不用错误区分“数据不足”和“未达阈值”；调用者需要更细分类时必须先通过需求扩展契约。

## 7. 性能与安全约束

单次匹配的主要成本是复制和排序，适用于每只证券有限季度财报；禁止传入无界全市场混合切片。

## 8. 测试与验收证据

测试覆盖旧行为兼容、累计值转单季、收入/利润增长、缺失季度、无效配置和输入不变性。

```bash
go test ./internal/financialscreen -cover
```

## 9. 相关文档

- [财报业务设计](../business.md)
- [旧数据访问设计](../data.md)
