---
kind: explanation
status: baseline-review
authority: code-derived
baseline_revision: de5a2120343878ba5078d2559e542463e4d67a88
owns: []
related: []
---

# 回测：一个策略在过去会怎样买卖、结果如何解释

你指定一只证券、一个策略及其参数、一个时间窗和一套执行假设（初始资金、费率、滑点、手数、持有期），系统按已入库行情逐根 Bar 模拟，返回订单、成交、交易、权益曲线和汇总指标。回测不下单，也不预测未来；它回答的是“这套规则在这段历史、这版数据上会发生什么”。

**先看结果含义，再深入实现：** [一个完整例子](#完整走一遍) · [信号与成交时刻](#信号何时成交) · [费用与滑点](#费用与滑点怎样计算) · [买卖多少](#买卖多少) · [持有期与退出](#持有期与退出) · [指标与结果解释](#结果怎样解释) · [失败与复查](#失败取消与复查) · [证据与待确认事项](#证据与待确认事项)。

本文解释当前代码；策略本身的判断条件见各策略文档（如 [日线 B1](../strategies/daily-b1.md)），完整技术契约见 [回测模块设计](../internal/backtest.md) 与 [应用层设计](../internal/application.md)。

## 完整走一遍

假设你请求：`SSE:600000`、`daily_b1_buy` 版本 1、默认参数，2025-01-01 到 2025-06-01，初始资金 100 万元，现金使用比例 100%，佣金 0.03%（最低 5 元），印花税 0.05%（仅卖出），滑点 0.05%，每手 100 股，持有期 10 根。以下价格均为假设的前复权输入，用来说明规则，不代表真实行情。

| 步骤 | 系统做什么 |
|---|---|
| 1. 创建 | 固定策略定义、参数（未传的用默认值）、请求时的最新完成行情版本、引擎版本；计算输入摘要并持久化任务，返回 `run_id` |
| 2. 排队执行 | Worker 从队列领取任务并获得租约，定期续租；执行前核对输入摘要与保存的一致，防止任务内容被改动 |
| 3. 加载数据 | 按固定版本读取日线（含 30 根预热）与复权因子；预热用于让指标在窗口起点就可用，账户从窗口起点开始 |
| 4. 逐根模拟 | 对窗口内每根日线依次：应用到期公司行动 → 尝试上一根收盘排的订单 → 按收盘价记录权益 → 执行策略决策 → 为下一根开盘排单 |
| 5. 原子发布 | 订单、成交、交易、权益曲线、汇总、任务终态和完成事件在同一事务写入 |

假设 D1（某日）收盘时策略发出买入信号，D2 开盘价 10.10 元：

- 买入价 = 10.10 × (1 + 0.0005) 向上取整到价格精度 = **10.1051 元**（若超出 D2 最高价则取最高价）。
- 买入数量：预算 = 现金 × 100% = 100 万元；按 10.1051 元能买 989 股，取整手 → **900 股**（若不足一手则整单拒绝）。
- 成交额 = 10.1051 × 900 = 9094.59 元；佣金 = ⌈9094.59 × 0.03%⌉ = 2.73 元，低于最低佣金 5 元，按 **5 元**收；买入无印花税。
- 合计支出 9099.59 元，从现金扣除。

之后策略在持有满 10 根或发出退出信号时卖出；卖出价 = 开盘价 × (1 − 0.0005) 向下取整（不低于当日最低价），并加收 0.05% 印花税。一笔交易记录买入支出、卖出净收入、持有期现金分红和净利润。

**最后一根日线的决策不会成交**：收盘后排在“下一根开盘”的订单如果没有下一根，会以 `UNFILLED_NO_NEXT_BAR` 结束并保留在订单列表里；持仓保留为期末持仓，权益按最后收盘价估值。

## 信号何时成交

信号在 Bar 收盘后产生，订单最早在**下一根**可交易 Bar 的开盘成交，绝不与本根收盘价成交。这来自全局不变量“信号在当前 Bar 收盘后产生，订单最早在下一根可交易 Bar 开盘成交”，目的是不使用产生信号时还不知道的价格。

由此推论：

- 追问“为什么 D1 有信号却按 D2 开盘价买”是正常语义，不是延迟 bug。
- 停牌或无成交量的 Bar 不能成交；订单在该 Bar 上被拒绝后的处理见 [持有期与退出](#持有期与退出)。
- 买入拒绝不会顺延重试：这一轮不建仓，等下一次信号。卖出拒绝则每根开盘重试，直到卖出成功。

## 费用与滑点怎样计算

全部金额用缩放整数计算，不经过浮点，保证同样输入得到同样结果。

| 项目 | 计算方式 | 方向 |
|---|---|---|
| 滑点 | 开盘价 × (10000±slippage_bps)/10000 | 买入向上取整、卖出向下取整，再夹在当日 `[low, high]` 内 |
| 佣金 | 成交额 × commission_bps，向上取整 | 买卖都收；低于最低佣金按最低 |
| 印花税 | 成交额 × stamp_duty_bps，向上取整 | **仅卖出** |
| 过户费 | 成交额 × transfer_fee_bps，向上取整 | 买卖都收 |

费用向上取整是保守选择：宁可多收一分不少收。卖出时若费用总额超过成交额，整单拒绝（`FEES_EXCEED_PROCEEDS`）。上例费率只是演示执行假设，不是费率建议。

## 买卖多少

- **买入**：目标是用掉可用现金的一定比例（`cash_fraction_bps`，默认请求给多少用多少），换算成不超过预算的最大**整手**数量。数量不足一手时整单拒绝（`INSUFFICIENT_CASH`），不会买半手。
- **卖出**：数量为 0 时表示清空全部持仓；卖出数量不能超过持仓。
- 当前回测**一次只模拟一只证券**，没有组合、对冲或融资；现金账户不允许透支，买入拒绝的原因里最常见的就是现金不够一手。

## 持有期与退出

引擎在每根收盘检查持仓状态，满足任一条件即为下一根开盘排卖单：

1. 策略主动退出（`ExitLong`）。
2. 上一次卖出被拒后的重试。
3. 持有根数达到 `hold_bars`（请求未传时用策略默认值，日线 B1 为 10 根），卖出原因记为 `default_hold_exit`。

持有根数从成交那根算起（成交根记第 1 根）。持仓期间新的买入信号被忽略；先退出再谈新建仓。一次完整交易（买入→卖出）的净利润 = 卖出净收入 + 持有期现金分红 − 买入总支出；持有根数与分红都记在交易里。

## 公司行动在回测里的位置

现金分红和送股按除权日应用在开盘前，直接改现金或持仓；分红计入持有期交易利润。配股权证当前不支持，遇到会报错。

**当前数据现实是：生产刷新不采集公司行动**（见 [行情采集](market-data.md)），所以现在跑出的回测不会有真实分红送股；这些路径是能力预留。把“回测没算分红”当成数据缺口，而不是引擎缺陷。

## 结果怎样解释

任务完成后可查：`GET /api/v1/backtest-runs/:run_id`（状态与汇总）、`.../orders`、`.../trades`、`.../equity`。汇总指标由引擎从权益曲线和交易列表计算：

| 指标 | 含义 | 边界 |
|---|---|---|
| `total_return` / `annualized_return` | 期末权益相对初始现金的收益；年化按 365 天折算 | 初始权益非正时不存在 |
| `maximum_drawdown` | 权益从峰值回落的最大比例 | 峰值非正时该段不计 |
| `win_rate` | 已平仓交易中净利润为正的比例 | 无平仓交易时不存在 |
| `profit_factor` | 盈利总额 ÷ 亏损总额 | 无亏损时不存在 |
| `average_holding_bars` | 平均每笔交易持有根数 | — |
| `closed_trades` / `has_open_position` | 平仓笔数与期末持仓 | 期末持仓按最后收盘价估值 |

权益曲线每根 Bar 记一点（现金 + 持仓按收盘价估值），起点是窗口首根开盘时点、等于初始现金。

**解读时的现实边界：** 指标反映的是“这套规则 + 这套执行假设 + 这版数据”的历史模拟。执行假设（尤其滑点与最低佣金）由请求方给定，结果不会告诉你假设是否现实；数据版本与复权口径的影响见 [行情采集](market-data.md)。不要把回测收益当成策略未来表现的预测。

复查一个结果：核对 `run_id` 固定的策略版本、参数、行情版本、引擎版本与时间窗；重跑“最新数据”得到的差异来自输入变化，不说明旧结果错误。

## 失败、取消与复查

| 你看到什么 | 应怎样理解 |
|---|---|
| 创建被拒绝 | 策略/版本不存在、参数非法、日期非法（须 UTC、范围最多 20 年）、配置非法（比例与费率 0–10000bps、手数>0、持有期≥0） |
| 任务长时间排队或失败 | Worker 失租后任务会被其他 worker 重新领取重试；确定性错误不重试 |
| `SUCCEEDED` 但没有交易 | 窗口内没有满足条件的信号；正常结果 |
| 引擎执行中取消 | 已发出的 HTTP 请求结束不取消任务；显式 cancel 在检查点生效，已取消任务不发布成功结果 |
| 订单 `UNFILLED_*` | 按原因分类：资金不足、无涨跌停数据外的不可交易、数量不足一手等；见 HTTP 契约字段表 |
| 同一幂等键重试 | 复用原任务；改请求内容同键会冲突 |

订单/成交/交易逐条持久化，幂等键防止同一请求创建多个任务；结果发布与任务终态在同一事务，不会出现“任务成功但订单缺一半”。

## 实现入口

| 想理解或修改什么 | 实现入口 | 影响边界 |
|---|---|---|
| 逐 Bar 顺序、订单生命周期 | [engine.go](../../../internal/backtest/engine.go)：`Run` | 顺序变化属于引擎语义，必须提升 `EngineVersion` |
| 成交价、费用、整手与拒绝 | [execution.go](../../../internal/backtest/execution.go)、[cost.go](../../../internal/backtest/cost.go) | 同上；费用取整方向影响所有策略结果 |
| 账户与公司行动 | [account.go](../../../internal/backtest/account.go)：`ApplyFill`、`ApplyCorporateActions` | 现金账户语义；无组合与融资 |
| 指标计算 | [metrics.go](../../../internal/backtest/metrics.go) | 汇总口径 |
| 创建固定输入、幂等 | [backtest_service.go](../../../internal/application/backtest_service.go)：`Create` | 输入摘要变化会使旧任务失效 |
| 窗口与预热切分、公司行动过滤 | [backtest_worker.go](../../../internal/application/backtest_worker.go)：`Execute`、`backtestWindow` | 指标含预热、账户从窗口起，两条边界在此汇合 |
| Worker 领取、续租、失败恢复 | [worker_pool.go](../../../internal/application/worker_pool.go) | 与扫描共用任务机制 |
| 结果原子发布 | [backtest_result_store.go](../../../internal/infrastructure/mysql/backtest_result_store.go) | 完整持久化契约归 [MySQL 设计](../internal/infrastructure/mysql.md) |

## 证据与待确认事项

| 要证明的行为 | 既有测试入口 | 验证边界 |
|---|---|---|
| 信号次根开盘成交、不与本根成交 | [engine_test.go](../../../internal/backtest/engine_test.go)：`TestEngineFillsSignalAtNextOpen`；[execution_test.go](../../../internal/backtest/execution_test.go)：`TestOrderDoesNotFillOnSignalBar` | 引擎层，不含排队与持久化 |
| 开盘滑点、整手、费用、不留负现金 | [execution_test.go](../../../internal/backtest/execution_test.go)：`TestBuyUsesNextBarOpenAndRoundsToBoardLot`、`TestBuyAccountsForSlippageAndEveryFeeWithoutNegativeCash`、`TestCashFractionLimitsBuyBudgetAndCeilingFeeIsDeterministic` | 确定性整数运算 |
| 买入拒绝不重试、卖出拒绝重试 | 同文件：`TestEntryRejectionExpiresButExitRejectionIsRetried` | — |
| 最后一根决策不成交 | [engine_test.go](../../../internal/backtest/engine_test.go)：`TestLastBarDecisionRemainsUnfilled` | — |
| 分红计入交易、送股不动现金 | 同文件：`TestEngineCountsCashDividendInClosedTradeMetrics`、`TestEngineDoesNotTreatShareDistributionAsCashDividend` | 引擎行为；当前无真实公司行动数据 |
| 涨跌停与不可交易拒绝 | [execution_test.go](../../../internal/backtest/execution_test.go)：`TestExecutionRejectsUntradableLimitsAndInsufficientCash` | 用构造数据验证；当前新浪数据无涨跌停价与停牌标记，实际不触发 |
| 取消不发布半成品 | 同文件：`TestEngineReturnsNoPartialResultWhenCancelled` | 引擎层 |
| 创建幂等、输入固定 | [backtest_service_test.go](../../../internal/application/backtest_service_test.go) | 服务层，不含真实 MySQL |

**需要你判断或留意的现状：** 是否为公司行动接回数据来源（否则分红路径始终空转）；单证券回测是否满足你的使用，组合与资金管理的需求是否存在；A 股当前数据没有涨跌停价与停牌状态，撮合的相应拒绝条件实际不会触发，是否需要补充采集。

本文依据本地 `de5a212` 代码整理；上例中的价格与成交数字是可复算的说明性假设，不是真实行情。
