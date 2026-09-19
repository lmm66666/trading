---
kind: explanation
status: baseline-review
authority: code-derived
baseline_revision: de5a2120343878ba5078d2559e542463e4d67a88
owns: []
related: []
---

# 底部倍量回撤：为什么这根日线会出现买入信号

底部倍量回撤寻找“长期低位先出现放量上涨（单日暴量或连续数日温和放量），随后有限度回撤”的形态。与 [日线 B1](daily-b1.md) 的差别是：候选必须从**低位区**启动，启动方式多一种“渐进式上涨”，回撤和过滤参数也更宽。当前策略 ID 为 `bottom_surge_pullback`，版本 `1`。

## 输入与输出

输入是同一证券、固定数据版本的一组日线及复权因子。策略声明 60 根预热（容纳 60 日最低价检查与 MA60）；默认持有期 10 根。每根日线输出“不买入”或“买入”，原因记为 `bottom_surge_pullback`。与日线 B1 一样没有退出动作，回测中由持有期或人工取消平仓。

## 如何判断信号

### 计算口径

涨幅、均线、KDJ、60 根低价均用前复权值；成交量与均量用原始值。量比与涨幅的定义和日线 B1 相同（量比含当天，涨幅相对上一根收盘）。

### 默认参数

| 你可以调整什么 | 默认值 | 准确含义 | 参数名 |
|---|---:|---|---|
| 低位带宽 | 15% | 当日开盘价 ≤ 最近 60 根日线最低价 × 1.15 才算“在低位区” | `low_band_pct` |
| 单日量比 | 2 | 单日启动：量比 ≥ 2 且涨幅 ≥ 5% | `single_volume_ratio` / `single_rally_pct` |
| 渐进天数 | 3 根 | 渐进启动：连续 3 根，每根量比 ≥ 1.2 且涨幅 ≥ 2% | `gradual_days` |
| 渐进量比/涨幅 | 1.2 / 2% | 同上 | `gradual_volume_ratio` / `gradual_rally_pct` |
| 延续间隔 | 3 根 | 跟进放量上涨距上次放量 ≤ 3 根时并入本轮上涨，不重置候选 | `surge_gap` |
| 最大回撤 | 20% | 相对跟踪峰值收盘的跌幅 ≤ 20% | `pullback_pct` |
| 最长回撤 | 15 根 | 当前与峰值日线的索引差 ≤ 15 | `pullback_bars` |
| J 值区间 | −20 到 20 | 当日 J 值必须落在 `[j_min, j_max]` 内（含边界） | `j_min` / `j_max` |

### 每来一根日线，依次做什么

1. **在低位区启动候选。** 两种方式：
   - **单日放量**：量比和涨幅同时达标，且当日开盘价在 60 根最低价带宽内（含当日）；以当天收盘为初始峰值。
   - **渐进放量**：连续 `gradual_days` 根每天量比与涨幅达标；“是否在低位区”在渐进的**第一根**判定，之后即使价格离开低位区也不影响计数。最后一根渐进日并入上涨。
   - 不在低位区时，放量上涨不会启动候选（RequireNearLow）。
2. **延续上涨。** 跟进的放量上涨若距上次放量 ≤ `surge_gap` 根，直接延长本轮上涨并更新峰值，**不重置候选**；这是与日线 B1（间隔为 0，新放量总是重启）的重要差别。间隔超过 `surge_gap` 的新放量才重新启动。
3. **常规更新峰值。** 收盘达到或超过峰值就更新峰值及其位置；与峰值持平也算上涨，不作为回撤。
4. **回撤窗口内发信号。** 收盘低于峰值时，回撤深度和时长都在上限内即满足回撤条件；随后的过滤通过才发信号。
5. **过滤：趋势与 J。** MA20 > MA60、收盘 ≥ MA60、J 值在区间内。任一不满足不发信号，但不清除候选。
6. **回撤超限失效**，等待下一次低位启动；超过 `surge_gap` 的新放量也会重启候选。

均线或 J 值无效时不发信号但不清除候选；收盘、上一收盘、成交量或均量无效时不推进跟踪。

## 用一个例子检查理解

假设输入（前复权收盘价；60 根最低价 10.00，日线开盘均在其 15% 带宽内）：

| 日线 | 收盘价与条件 | 系统怎样理解 | 当日买入信号 |
|---|---|---|---|
| D0 | 10.2，量比 2.3，涨幅 6% | 单日放量且在低位区，建立候选，峰值 10.2 | 无（启动日不发回撤信号） |
| D1 | 10.6 | 更新峰值 10.6 | 无 |
| D2 | 10.4；MA20 > MA60，收盘 ≥ MA60，J=5 | 回撤约 1.9%，1 根；过滤通过 | **有** |
| D3 | 10.3，量比 2.1，涨幅 4%（不足 5%） | 不是新的单日启动（涨幅不够），作为常规收盘更新 | 仍可能有 |
| D4 | 10.2 | 回撤约 3.8%；候选继续 | 视过滤而定 |

若 D2–D4 中某天回撤达到 20% 或距峰值 16 根，候选失效，需要新的低位启动。

| 边界 | 当前行为 |
|---|---|
| 渐进第 3 根才满足天数 | 第 3 根并入上涨，此后的下跌才进入回撤判断 |
| 低位区判定时点 | 单日启动看当天开盘；渐进启动看第一根收盘日的低位状态 |
| 跟进放量恰好距上次 3 根 | 并入本轮（≤ surge_gap）；第 4 根才重启 |
| J 恰好等于 −20 或 20 | 发信号（含边界；与日线 B1 的“严格小于”不同） |
| 回撤深度恰好 20% | 候选仍有效 |

## 实现与修改影响

| 想理解或修改什么 | 实现入口 | 影响边界 |
|---|---|---|
| 判断顺序、默认参数 | [bottom_surge.go](../../../internal/strategy/builtin/bottom_surge.go)：`OnBar`、`bottomParameterSpecs` | 修改默认行为需要新策略版本 |
| 低位区、渐进、延续间隔、回撤状态机 | [pullback_tracker.go](../../../internal/strategy/builtin/pullback_tracker.go)：`Advance`、`advanceIdle` | 与日线 B1 共享跟踪器；改动须同时检查另一策略 |
| 60 根最低价带宽 | 同文件 `isNearSixtyBarLow` | — |
| 均量是否包含当天 | [sma.go](../../../internal/indicator/sma.go)：`smaSeries` | 共享计算，不能单独为本策略改变 |
| 实例隔离与禁止未来数据 | [策略模块设计](../internal/strategy.md) | 全部策略共用 |

默认数值的研究依据现有证据不足以还原，只记录默认值。

## 验证依据

| 要证明的行为 | 证据 | 验证边界 |
|---|---|---|
| 定义、默认参数与整数约束 | [bottom_surge_test.go](../../../internal/strategy/builtin/bottom_surge_test.go)：`TestBottomSurgeDefinitionCarriesCompleteDefaultContract`、`TestBottomSurgeRejectsFractionalAndOutOfRangeParameters` | 定义层 |
| 离开低位区后跟进放量仍可延续上涨 | 同文件：`TestBottomSurgeExtendsQualifiedFollowOnSurgesAfterLeavingLowBand` | 跟踪器层 |
| 追加未来数据不改变既有信号 | [prefix_test.go](../../../internal/strategy/builtin/prefix_test.go)：`TestBottomSurgePastSignalsDoNotChangeWhenFutureBarsAreAppended` | 合成样本 |
| 回撤深度/超时失效、重启 | [pullback_tracker_test.go](../../../internal/strategy/builtin/pullback_tracker_test.go)：`TestPullbackTrackerExpiresWhenDepthOrDurationIsExceeded`、`TestPullbackTrackerRestartsAfterExpiredWindow` | 与日线 B1 共享 |
| 本文中 D0–D4 示例及各等号边界 | 上述源码静态核对 | 未新增为自动化用例 |

本文依据本地 `de5a212` 代码整理，见 [本批验证记录](../../changes/archive/2026-09-19-readable-design-continuation/verification.md)。
