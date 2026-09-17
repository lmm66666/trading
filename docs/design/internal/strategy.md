---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["internal/strategy/", "internal/strategy/builtin/"]
related: []
---

# 策略模块设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `internal/strategy`、`internal/strategy/builtin` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

本模块定义编译型策略的静态契约、参数、注册与解析、运行时 Context、跨周期 Timeline 和严格按 Bar 回放的信号决策。`builtin` 提供版本化内置策略。

本模块不读取行情仓储、不决定任务版本、不管理资金、不撮合订单，也不持久化信号。

## 2. 对外能力与使用者

- `Definition` 声明策略 ID、版本、主周期、预热根数、指标依赖、辅助周期、默认持有期和参数规格。
- `Registry` 注册工厂、列出稳定定义，并按 ID/版本/参数创建全新策略实例。
- `Timeline` 组合主周期 Dataset、指标集合和只向历史对齐的辅助周期。
- `Context` 只暴露当前 Bar、当前位置和当前或历史特征值。
- `ReplayLatest` 顺序回放一个实例并返回最后决策。

扫描服务、回测引擎、策略目录 API 和图表/计算编排使用这些能力。

## 3. 依赖和边界

只依赖 `internal/market`、`internal/indicator` 和 Go 标准库。策略实例不能获得 Repository、HTTP 客户端、系统时钟或完整未来切片。

应用层负责锁定 Definition、参数、数据与引擎版本；策略模块负责验证这些输入彼此兼容。

## 4. 核心模型与不变量

### 4.1 定义与版本

- ID、Version、主周期必填；预热和默认持有期不得为负。
- 指标引用必须合法、键不重复，且周期属于主周期或已声明辅助周期。
- 参数名非空，默认值和用户值必须有限、位于闭区间；整数参数不得接受小数。
- 同一策略行为改变必须新增策略版本。版本 1 当前包含 `daily_b1_buy`、`weekly_b1_buy`、`bottom_surge_pullback`。

### 4.2 Registry 与实例隔离

- `(ID, Version)` 在 Registry 内唯一，未知或重复注册失败。
- Registry 防御性复制定义、参数规格和传入参数。
- 每次 Resolve 调用工厂并返回独立实例；扫描不同证券不得共享可变策略状态。
- 工厂返回 nil、改变静态定义或产生非法定义时解析失败。

### 4.3 时间线与 Context

- 主/辅助 Dataset 必须合法，并属于相同证券和数据版本。
- 辅助索引必须是主 Bar 截止时最新且已收盘的 Bar，不能映射未来值或更旧替代值。
- `Float(ref, ago)` 只能读取当前索引及历史，负 ago、越界或无效指标会设置 Context 错误或返回无效。
- Timeline 防御性复制特征映射和对齐关系，调用者不能在回放中篡改输入。

### 4.4 决策

策略动作仅为 Hold、EnterLong、ExitLong。策略必须先检查指标有效位，再作判断。扫描回放中的 PositionView 为空；回测 Context 由引擎提供真实持仓视图。

## 5. 主要流程

```text
Registry 固定定义
  → 校验并补齐参数
  → 创建单次、单证券策略实例
  → 构造 Timeline 和特征集合
  → 从最早 Bar 顺序调用 OnBar
  → 返回决策或交给回测引擎排单
```

内置策略的金样本和前缀测试定义当前版本行为；不能在相同版本下静默改变判断条件、预热、参数默认值或跨周期对齐方式。

## 6. 失败、取消和一致性语义

- Definition、参数、Timeline 或特征不兼容时，在调用策略逻辑前失败。
- OnBar 返回错误或尝试未来/越界访问时，Replay 立即失败，不返回看似有效的最后信号。
- Replay 不自行处理 context；长批量任务在应用层和回测 Bar 边界实施取消。

## 7. 性能与安全约束

- 单次 Replay 对 Bar 顺序遍历，策略内部不得访问外部系统或执行无界工作。
- 指标在进入策略前统一构建，策略不为每根 Bar 重算完整历史。
- 参数与定义复制避免调用者共享可变 map/slice 引起跨任务污染。

## 8. 测试与验收证据

测试覆盖非法定义全集、Registry 实例隔离、参数边界、Timeline 防御性复制、未来数据拒绝、辅助周期 as-of 对齐、内置策略金样本和前缀不变性。

```bash
go test ./internal/strategy/... -cover
```

合并覆盖率门槛为 90%。

## 9. 相关文档

- [系统设计](../../architecture/system-design.md)
- [指标设计](indicator.md)
- [回测设计](backtest.md)
- [应用层设计](application.md)
