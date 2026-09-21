---
kind: explanation
status: baseline-review
authority: code-derived
baseline_revision: de5a2120343878ba5078d2559e542463e4d67a88
owns: []
related: []
---

# 图表查询：前端怎样拿到一页行情和指标

图表查询回答“这只证券最近（或某时点之前）的 K 线长什么样、指标值是多少”。它是只读链路：不产生任务、不写库，每次请求独立完成。你看到的每一页都绑定一个数据版本，向前翻页不会因为新数据发布而错位。

**入口：** [一个完整例子](#完整走一遍) · [分页](#怎样向前翻历史) · [指标](#指标怎样计算) · [失败与边界](#失败与边界) · [证据与待确认事项](#证据与待确认事项)。

## 完整走一遍

你在图表页搜索并选中 `SSE:600000`，日线、前复权（QFQ）、MA20 和 KDJ(9)。前端发出：

```text
POST /api/v1/chart-queries
{"instrument":"SSE:600000","timeframe":"DAY","price_view":"QFQ",
 "indicators":[{"kind":"SMA","period":20},{"kind":"KDJ","period":9}]}
```

系统依次执行：

| 步骤 | 系统做什么 | 值得注意什么 |
|---|---|---|
| 1. 校验 | 证券身份、周期（DAY/WEEK）、视图（RAW/QFQ）、limit（100–1000，缺省 400）、指标个数与参数 | 指标最多 16 个；KDJ/均线周期 1–500 |
| 2. 校验证券 | 查证券目录，确认存在且**处于激活状态** | 未激活或未知证券直接失败，不返回空数据 |
| 3. 解析版本 | 请求未传 `data_version` 时取当前最新完成版本 | 传了版本就固定用那个版本，不复解析 |
| 4. 读取 | 按固定版本读该证券该周期**最长 20 年**的历史（按修订规则取版本视角） | 不是只读你要的那一页，见 [指标](#指标怎样计算) |
| 5. 截取 | 取出**最后** `limit` 根作为本页；更早的根被丢弃但已参与指标计算 | 返回页按时间升序排列 |
| 6. 换算价格 | QFQ 视图按因子换算每根 Bar；RAW 直接输出 | 缺因子的 Bar 会导致整次查询失败，不静默跳过 |
| 7. 计算指标 | 在完整历史上构建指标序列，再同样截取最后一页的有效点 | 预热期内的无效点直接不输出 |
| 8. 返回 | K 线页、指标序列、`data_version`、`has_more`、`next_before` | — |

前端拿到后渲染 K 线与指标；翻页时携带 `next_before` 继续请求。切换证券或参数时，前端会中止（abort）上一条未完成请求，旧响应不会覆盖新查询。

## 怎样向前翻历史

`next_before` 是本页最早一根的收盘时刻。翻页时把它作为下一页的 `before` 游标；服务端把它减 1 微秒后作为读取上界，因此**不会重复也不会跳过**任何一根。`has_more` 表示更早历史仍然存在。

由此推论：

- 每一页都返回 `data_version`；只要前端一直传回它最早解析到的版本，整个翻页过程看到的是同一版本视角。若不传版本而中途发布了新数据，新页会用新版本——窗口内修订过的证券可能出现页间接缝。
- 指标序列与 K 线页同源同时计算，不存在“这页 K 线配上一页指标”。

## 指标怎样计算

支持的指标：`SMA`、`EMA`、`MACD`（`fast`/`slow`/`signal`，返回 DIF、DEA、Histogram 三条）、`KDJ`（`period`，返回 K、D、J 三条），以及 `ZSCORE`（双价格相对偏差，返回柱、平滑、长期三条）。

关键语义：**指标在截取本页之前的完整历史上计算**。请求 MA20 时，即使本页只有 400 根，服务端也从 20 年历史的第一根可用数据开始累积。所以本页第一根的 MA20 是真实的历史均值，不是用页内数据重新预热的结果；这与回测“预热后开窗”的思路一致（见 [回测流程](backtesting.md)）。

为防止一次请求算爆，有两条预算：指标成本总和 ≤ 2000（SMA/EMA 每个记 1，MACD 记 3，KDJ 记 `period × 3`，ZSCORE 记 `2*period + regime + 68`），以及进程内同时进行的指标构建最多 4 个，超出的请求排队等待。重复参数组合（同 kind 同参数）的指标会被拒绝而不是去重。

## 失败与边界

| 你看到什么 | 应怎样理解 |
|---|---|
| 请求被拒绝 | 参数非法（周期/视图/limit/指标参数/重复指标/成本超限）或证券身份无法解析 |
| 证券数据不存在 | 该证券在该版本下无行情，返回失败而非空页 |
| QFQ 查询失败 | 某根 Bar 找不到生效因子——数据缺口，不静默跳过 |
| 指标序列某点缺失 | 对应位置处于预热期（无效值），属正常输出 |
| `has_more=false` | 已到该证券该版本下的最早数据 |

图表查询不感知扫描/回测任务；它每次独立读取。想要“当时看到的版本”就用扫描或回测任务里固定的版本号。

## 实现入口

| 想理解或修改什么 | 入口 | 详细技术规则 |
|---|---|---|
| 校验、版本解析、截取、预算 | [chart_query_service.go](../../../internal/application/chart_query_service.go)：`Query`、`validateChartQuery` | [应用层设计](../internal/application.md) |
| 指标组装（MACD/KDJ 展开为多条序列） | 同文件：`chartIndicatorRefs` | [指标设计](../internal/indicator.md) |
| HTTP 请求与响应字段 | [query_chart.go](../../../api/query_chart.go) | [HTTP 契约](../../standards/http-api.md) |
| 前端分页与请求中止 | [ChartWorkspace.tsx](../../../web/src/features/chart/ChartWorkspace.tsx) | [前端设计](../web.md) |
| 版本视角读取 | [market_data_repo.go](../../../internal/infrastructure/mysql/market_data_repo.go)：`Dataset` | [MySQL 设计](../internal/infrastructure/mysql.md) |

## 证据与待确认事项

| 要验证什么 | 既有测试入口 | 能证明到哪里 |
|---|---|---|
| 校验边界、成本预算、并发上限 | [chart_query_service_test.go](../../../internal/application/chart_query_service_test.go) | 服务层 |
| 前端翻页用 next_before、切换时中止旧请求 | [ChartWorkspace.test.tsx](../../../web/src/features/chart/ChartWorkspace.test.tsx) | 交互行为 |
| 请求/响应字段 | [query_chart_test.go](../../../api/query_chart_test.go) | 传输层 |

本文依据本地 `de5a212` 代码整理，见 [本批验证记录](../../changes/archive/2026-09-19-readable-design-continuation/verification.md)。

## 怎样理解涨幅偏差

添加“Z-score 126,5,252”后，选择关联期货，副图衡量股票与期货价格比值偏离历史均值的标准差数。仅支持日线，期货缺日时使用最近已知收盘价，显示实际商品日期。平滑线是主Z的EMA，长期线是更长窗口的比值Z；附相对表现、相关性和状态。完整契约见[指标设计](../internal/indicator.md#双价格相对-z-score)。旧STD/RETZ在本地草稿替换，手动保存后落库。
