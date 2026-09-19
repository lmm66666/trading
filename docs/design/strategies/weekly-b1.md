---
kind: explanation
status: baseline-review
authority: code-derived
baseline_revision: de5a2120343878ba5078d2559e542463e4d67a88
owns: []
related: []
---

# 周线 B1：为什么这根周线会出现买入信号

周线 B1 在一根**周线**收盘后，检查周线级别的“三线归位”：KDJ 的 J 值足够低、周线 MA20 位于 MA60 之上、收盘价站上 MA60 且不低于日线 MA20。全部满足即发出买入信号。当前策略 ID 为 `weekly_b1_buy`，版本 `1`。

它没有回撤跟踪过程，是三个内置策略中唯一的“对齐即入”策略；扫描窗口、任务和结果含义统一见 [策略扫描流程](../workflows/strategy-scan.md)，日线 B1 的回撤形态见 [日线 B1](daily-b1.md)。

## 输入与输出

主周期是周线；同时声明日线 MA20 为辅助数据。回测与扫描会同时加载两个周期（含 60 根周线预热，预热大约覆盖一年半的周线）。每根周线到来时输出“不买入”或“买入”；买入原因记为 `weekly_b1_alignment`。默认持有期 10 根周线，供回测使用；策略本身没有退出动作。

## 如何判断信号

价格与均线使用前复权值；J 值计算口径（9 周期 KDJ、K/D 初值 50、`J = 3K − 2D`）与日线 B1 相同，只是作用在周线上。

每根周线收盘后依次检查，**任何一步不满足都直接不买**：

1. **J 值低于阈值。** 当前周 J 值 < `kdj_threshold`（默认 10；这是本策略唯一可调参数）。
2. **均线多头。** 周 MA20 > 周 MA60。
3. **价格站上长期均线。** 收盘价 ≥ 周 MA60。
4. **日线过滤。** 收盘价 ≥ 日线 MA20。这里用的日线 MA20 是**周线收盘时刻或之前**最新一根日线上的值——不是未来日线，也不是周内任意一天的值。

指标值无效（预热不足）时不发信号。由于周线只发布已结束的 ISO 周（见 [行情采集](../workflows/market-data.md)），信号最早出现在一周结束、周线入库之后。

## 用一个例子检查理解

以下为说明规则的假设输入；均线和 J 值视为已计算的输入，不声称仅凭表中数字能推导它们。

| 周线 | 关键值 | 系统怎样理解 | 买入信号 |
|---|---|---|---|
| W0 | J=25，MA20=12.2 > MA60=12.0，收盘 12.5 ≥ MA60，日线 MA20=12.4 | J 太高 | 无 |
| W1 | J=8，其余同上，收盘 12.5 ≥ 日线 MA20=12.3 | 四项全部满足 | **有**（`weekly_b1_alignment`） |

| 边界 | 当前行为 |
|---|---|
| J 恰好等于 10 | 不发信号（要求严格小于） |
| 收盘恰好等于日线 MA20 | 发信号（允许等于） |
| 周 MA20 恰好等于周 MA60 | 不发信号（要求严格大于） |
| 周线收盘后日线又跌、日线 MA20 下移 | 不影响本周已发出的信号；影响下一根周线 |

**与日线 B1 的关键差异：** 没有放量启动、峰值跟踪和回撤窗口；一根周线满足对齐就发信号。因此它也不存在“同一轮回撤连续发信号”的问题——每根周线独立判断，连续多周满足条件会连续多周发信号。

## 实现与修改影响

| 想理解或修改什么 | 实现入口 | 影响边界 |
|---|---|---|
| 判断顺序、J 阈值、默认值 | [weekly_b1.go](../../../internal/strategy/builtin/weekly_b1.go)：`OnBar`、`weeklyParameterSpecs` | 修改默认行为需要新策略版本 |
| 辅助周期对齐（日线值取周收盘时刻或之前） | [backtest_worker.go](../../../internal/application/backtest_worker.go)：`buildComputeTimeline` 的映射；扫描同理 | 对齐规则变化影响所有用辅助周期的策略 |
| KDJ 与均线计算 | [kdj.go](../../../internal/indicator/kdj.go)、[sma.go](../../../internal/indicator/sma.go) | 共享指标，不能只为周线 B1 静默改动 |
| 实例隔离与禁止未来数据 | [策略模块设计](../internal/strategy.md) | 全部策略共用的技术契约 |

阈值 10 为什么最初这样选，现有证据不足以还原研究依据，因此只记录默认值。

## 验证依据

| 要证明的行为 | 证据 | 验证边界 |
|---|---|---|
| 声明日线辅助依赖与预热 | [weekly_b1_test.go](../../../internal/strategy/builtin/weekly_b1_test.go)：`TestWeeklyB1DefinitionDeclaresDailyAsOfDependency` | 定义层，不是行情场景 |
| 日线值取周收盘时刻或之前，不取未来 | 同文件：`TestWeeklyB1UsesDailyValueAtOrBeforeWeeklyClose` | — |
| 全部条件满足才入场 | 同文件：`TestWeeklyB1GoldenFixtureEntersWhenAllConfirmedFeaturesAlign` | 合成样本 |
| 追加未来数据不改变既有信号 | [prefix_test.go](../../../internal/strategy/builtin/prefix_test.go)：`TestDailyB1PastSignalsDoNotChangeWhenFutureBarsAreAppended` 所用的同一机制 | 合成样本，不代表市场收益验证 |

本文例子未新增自动化用例，属静态核对。代码基线 `de5a212`，见 [本批验证记录](../../changes/archive/2026-09-19-readable-design-continuation/verification.md)。
