---
id: CHG-2026-09-21-RETURN-ZSCORE-DESIGN
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T00:00:00+08:00"
approved_revision: "sha256:b7d02837cea8a32988e4d1c3f5744bfb38cbad0c77cc9d9f31599c7883b6803d"
approved_scope: ["REQ-RETZ-001", "REQ-RETZ-002", "REQ-RETZ-003", "REQ-RETZ-004", "REQ-RETZ-005"]
---

# 每日涨幅偏差（RETZ）副图指标 design

目标：在图表查询链路新增 RETZ 指标，计算单标的收盘价对数日涨幅的滚动 Z-Score，前端以 TV 参考面板风格渲染为单 pane 三分量副图。计算约定与 `investment-research/tools/tradingview/commodity-equity-divergence/indicator.pine` 保持一致（滚动 SMA 均值、总体标准差、EMA 平滑、长窗口状态、±1/±2σ 阈值）。

## Current and target behavior

当前：图表指标仅 STD/SMA/EMA/MACD/KDJ；STD 输出收盘价绝对标准差，无法表达涨幅偏离。前端副图按 `kind+参数` 身份分 pane，MACD/KDJ 已示范单指标多分量同 pane。

目标：请求 `{"kind":"RETZ","period":126,"smooth":5,"regime":252}` 时，响应返回同一稳定 Key 前缀下的三条 series（component 为 `histogram`/`smooth`/`regime`），前端在同一副图 pane 渲染阈值着色柱 + 两条线 + ±1/±2/0 参考线，图例显示 σ 值。

## Data sources, model, and flow

### 指标计算（`internal/indicator`，新增 `return_zscore.go`）

1. 输入：Build 图内按 `Timeframe/PriceView` 已换算的 Close 基础序列（复权因子由既有链路保证同版本有效）。
2. 对数日涨幅：`r_t = ln(c_t / c_{t-1})`；`t=0`、任一端点无效或收盘价 `≤ 0` 时 `r_t` 无效。
3. 滚动 Z（窗口 `w`）：`mean = SMA(r, w)`，`std = pstdev(r, w)`（总体口径，复用 `stddevSeries` 的中心化两遍算法）；窗口未填满、窗口内含无效点或 `std ≤ 1e-12` 时无效；`z_t = (r_t − mean) / std`。
4. 分量：`histogram = z(band)`；`regime = z(regime)`；`smooth = EMA(histogram, smooth)`，自 histogram 首个有效点起按既有 `emaSeries` 有效位语义传播，无效点不产出伪值。
5. 前缀不变性：全部为只回望滚动计算，EMA 播种点由历史起点唯一确定；追加未来 Bar 不改变既有前缀（纳入测试）。

### Ref 扩展（`ref.go`/`graph.go`）

- 新增 `RETZKind = "retz"` 与 `Field` 常量 `Smooth`、`Regime`（`Histogram` 复用）。
- 参数映射：`Period = band`，新增 `Ref.Smooth`、`Ref.Regime` 两个命名字段；`Fast/Signal` 对 RETZ 必须为 0。
- `Key()`：`retz/<timeframe>/<view>/<field>/p=<band>/sm=<smooth>/rg=<regime>`，包含全部影响计算的参数。
- `Validate`：RETZ 要求 `band ≥ 2`、`smooth ≥ 1`、`regime > band`，字段限 `Histogram/Smooth/Regime`，拒绝成交量字段。
- `graph.go compute()` 新增分派：同一 Ref 集合内 band/regime 不同的涨幅序列与中间量按稳定 Key 去重计算。

### 应用层（`chart_query_service.go`）

- 新增 `IndicatorRETZ = "RETZ"`；`IndicatorRequest` 增加 `Smooth int json:"smooth,omitempty"`、`Regime int json:"regime,omitempty"`。
- 校验：`band(period)` 2–500、`smooth` 1–500、`regime` 2–500 且 `regime > band`，`Fast/Signal` 必须为 0；重复判断的 seen key 扩展纳入 `smooth/regime`；成本 `band + regime`。
- `chartIndicatorRefs` 展开三条 descriptor：`histogram/smooth/regime`（同 MACD 模式）。

### 前端（`web/src`）

- `api/client.ts`：`IndicatorRequest` 联合新增 `{kind:'RETZ'; period:number; smooth:number; regime:number}`；`ChartSeries.kind` 接受 `'RETZ'`。
- `features/chart/boards.ts`：`validIndicator` 接受 RETZ 并校验参数边界与 `regime > band`。
- `features/chart/chartData.ts`：`indicatorLabel`（如 `涨幅Z(126,5,252)`）、`indicatorIdentity` 纳入三分量同身份。
- `features/indicators/IndicatorManager.tsx`：新增预设 `RETZ(126,5,252)`、参数编辑（band/smooth/regime，含 regime > band 校验）与成本公式 `band + regime`，错误文案与后端一致。
- `features/chart/FinancialChart.tsx`：
  - `histogram` 分量走既有 HistogramSeries，但 RETZ 改为逐点颜色：`z ≥ 2` 橙（`#fb923c` 系）、`z ≤ −2` 青（`#22d3ee` 系）、其余银（`#c0c4cc` 系）；按分量固定配色规则，不占调色板轮转。
  - `smooth` 固定浅银色细线（线宽 1），`regime` 固定紫色（`#C084FC`）细线；不做 TV 式逐段变色（lightweight-charts LineSeries 不支持分段颜色，见 Alternatives）。
  - pane 参考线：`+2/+1` 橙色虚线、`0` 银色实线、`−1/−2` 青色虚线，挂在 histogram series 的 `createPriceLine`；非 overlay 指标自动落新 pane 的规则不变，三分量同 pane。
  - 图例：分量值两位小数 + `σ` 后缀；主图百分比归一化逻辑不触及（副图保留原单位）。
- `api/mock.ts`：演示数据补 RETZ 计算（同公式），保持 dev 模式可用。

## Interface and contract changes

- `POST /api/v1/chart-queries` 请求体 `indicators[]` 新增 `kind:"RETZ"` 与可选字段 `smooth`、`regime`；响应 `series[].component` 新增取值 `smooth`、`regime`（`histogram` 已存在）。其余字段、分页游标、错误语义不变。
- 看板 `t_chart_boards.config.indicators` 可持久化 RETZ 项；读写仍由服务端统一校验（`validConfig` 对应后端校验同口径更新）。

## Errors, degradation, and recovery

- 参数非法（边界、`regime ≤ band`、多余字段）、重复指标、成本超限：整个请求失败，沿用 `ErrInvalidRequest` 映射，不返回部分序列。
- 预热期、非正价格、零方差窗口：对应点无效不下发，前端自然留空，不补零不前填。
- QFQ 缺因子、证券未激活等既有失败路径不变。

## Concurrency, capacity, performance, and security

- 计算复杂度 O(N × (band + regime)) 级别（两遍滚动），与 STD 同阶；成本公式 `band + regime` 把上限纳入既有 2000 预算；构建并发上限 4 不变。
- 无新增持久化、无新外部依赖、无凭据处理；浮点仅用于指标计算，不改 Price/Money 定点语义。

## Compatibility, migration, and rollback

- 纯增量：旧 kind 请求与旧看板配置行为不变；无数据库迁移。回滚即还原本变更代码，已存看板中的 RETZ 项在新旧校验下会被拒绝并在前端预检剔除，不导致崩溃。

## Alternatives and tradeoffs

- **复用 STD 前端换算**：STD 是绝对单位且无均值中心化，语义不符，否决。
- **每周期一个独立指标**：实现最简但每窗口占一个 pane，偏离 TV 参考形态，否决（用户已选单指标多分量）。
- **smooth 线逐段阈值变色**：lightweight-charts LineSeries 不支持分段着色，需拆多条重叠 series，复杂度不成比例；采用固定色线，柱子已承载阈值着色信息。
- **±1~±2 区间填充带**：库无原生两线间填充，需自绘 primitive；列为非目标。
- **简单涨幅 `(c/c₋₁)−1` 替代对数涨幅**：小涨跌幅下几乎等价，但对数口径与参考面板及其 Python 参考实现一致，且涨跌对称，采用对数。

## Test strategy

- `internal/indicator`：手算序列核对 histogram/regime（总体标准差口径）、smooth 的 EMA 传播；预热/非正价格/零方差无效位；前缀不变性；Ref 校验与 Key 稳定性；去重。
- `internal/application`：RETZ 校验边界、重复拒绝、成本（126+5+252 记账）、三分量展开与分页裁剪。
- `api`：请求/响应字段（含 `smooth`/`regime`）与非法 kind 拒绝。
- 前端 Vitest：`validIndicator`/`validConfig`、`indicatorLabel`/`indicatorIdentity`、IndicatorManager 预设与成本、FinancialChart 逐点颜色与参考线、mock 数据形状。
- 门禁：`npm --prefix web run check && go test ./... && go vet ./...`、`bash scripts/verify.sh`；指标包覆盖率 ≥90%。未触发的外部门禁记录不适用理由。

## Affected current designs, standards, and ADRs

验收后回填：[指标设计](../../../design/internal/indicator.md)（RETZ 职责与不变量）、[应用层设计](../../../design/internal/application.md)（校验/成本/展开）、[API 设计](../../../design/api.md) 与 [HTTP 契约](../../../standards/http-api.md)（新 kind 与字段）、[前端设计](../../../design/web.md)（渲染与图例约定）、[图表查询](../../../design/workflows/chart-query.md)（支持指标清单）。无新增 ADR。

## Approval scope and evidence

用户于 2026-09-21 随 [requirements](requirements.md) 一并批准本设计，批准内容版本见 frontmatter `approved_revision`。实现、验证并记录 verification；语义修订需重新批准。
