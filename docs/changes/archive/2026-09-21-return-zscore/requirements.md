---
id: CHG-2026-09-21-RETURN-ZSCORE
status: implemented
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T00:00:00+08:00"
approved_revision: "sha256:1dbab3d9df8d05c348252f53c241a0224ef32177fd4f0343147858723150072c"
approved_scope: ["REQ-RETZ-001", "REQ-RETZ-002", "REQ-RETZ-003", "REQ-RETZ-004", "REQ-RETZ-005"]
---

# 每日涨幅偏差（RETZ）副图指标 requirements

## Problem and user scenarios

图表副图现有 STD 是收盘价绝对标准差（元），无法回答「今天这根涨幅相对这只证券自身常态算不算异常」。用户参考 TradingView 上的「股商相对Z」面板（`investment-research/tools/tradingview/commodity-equity-divergence/indicator.pine`），希望在行情工作台副图用同一套滚动 Z-Score 计算约定，展示**单标的每日涨幅相对其滚动常态的偏差**（无量纲 σ 值），形态为：短窗口 Z 柱 + 平滑线 + 长窗口状态线 + ±1/±2σ 参考线。

显式假设：参考面板的输入是双标的对数价差，本变更按用户「展示每日涨幅的偏差」的要求，把同一套计算约定（滚动 SMA 均值 + 总体标准差 + EMA 平滑 + 长窗口状态）应用到**单标的收盘价对数日涨幅**上，不做双标的相对价差。

## Environment, frequency, and data volume

- 链路：既有图表查询 `POST /api/v1/chart-queries`，只读、无任务、无写库。
- 数据量：每次请求读取固定版本最长 20 年历史（日线约 4800 根），响应页 100–1000 根；指标在完整历史上计算后裁剪。
- 频率：用户交互触发（切换证券/指标/分页），单用户个人工具，进程内指标构建并发上限 4 不变。
- 预算：指标总数 ≤16、总成本 ≤2000 不变，新指标成本计入同一预算。

## Requirements

- `REQ-RETZ-001`：新增图表指标 RETZ。输入为按请求价格视图换算后的收盘价序列，逐点计算对数日涨幅 `r_t = ln(c_t / c_{t-1})`（首个点、无效或非正收盘对应点无效）。主分量（histogram）`z_t = (r_t − SMA(r, band)) / pstdev(r, band)`，均值用滚动简单均值，标准差为**总体**口径、中心化两遍计算；窗口未填满、窗口含无效点或 `std ≤ 1e-12` 时该点无效且不输出。
- `REQ-RETZ-002`：RETZ 为单指标三分量，与 MACD 同模式展开：`histogram`（band 窗口 Z 值）、`smooth`（对 histogram 序列做 `smooth` 期 EMA，自首个有效点起按既有 EMA 语义传播）、`regime`（同一涨幅序列按 `regime` 窗口计算的 Z 值）。三分量共享同一指标身份，渲染于同一副图 pane。
- `REQ-RETZ-003`：参数与预算。`band`（对应请求字段 `period`）2–500、`smooth` 1–500、`regime` 2–500 且 `regime > band`，不满足即整个请求失败；相同 kind+参数组合视为重复指标拒绝；单个 RETZ 成本 = `band + regime`，计入 2000 总预算。默认预设 `band=126, smooth=5, regime=252`（与参考面板一致，约半年/平滑一周/一年）。
- `REQ-RETZ-004`：前端按 TV 参考风格渲染。histogram 逐点阈值着色：`z ≥ +2` 橙色、`z ≤ −2` 青色、其余银色；smooth 为固定浅色细线；regime 为固定紫色细线；pane 内绘制 +2/+1 橙色虚线、0 银色实线、−1/−2 青色虚线；图例展示各分量两位小数值并带 `σ` 后缀。
- `REQ-RETZ-005`：不破坏既有图表行为：指标序列满足前缀不变性；分页按稳定 key 合并去重；看板 `BoardConfig.indicators` 可保存/校验/恢复 RETZ 配置；指标变更仍触发完整首屏重查。

## Acceptance criteria

- Go 单测：给定人工可算序列，histogram/smooth/regime 输出与总体标准差口径手算结果一致；预热、非正价格、零方差窗口无效位正确；追加未来 Bar 不改变既有前缀；Ref 校验与稳定 Key 覆盖参数全集。
- 应用层/API 测试：参数边界（regime ≤ band、越界、多余字段）、重复拒绝、成本记账（band+regime）、三分量展开与裁剪分页正确。
- 前端测试：IndicatorRequest 联合类型与 `validIndicator`/`validConfig` 接受合法 RETZ、拒绝非法；`indicatorLabel`/`indicatorIdentity` 稳定；IndicatorManager 预设与成本提示正确；FinancialChart 对 histogram 逐点着色与参考线创建正确。
- 门禁：`npm --prefix web run check`、`go test ./...`、`go vet ./...`、`bash scripts/verify.sh` 通过；指标包覆盖率 ≥90%。

## Scope and non-goals

- 不做双标的「股商/股指相对 Z」（`RelativePriceZScore` 纯函数维持现状，不接入图表）；不做参考面板中仅数据窗口的 63 期相对表现、收益相关系数；不做仪表盘表格、状态文案与告警；不做 ±1~±2 区间填充色带（lightweight-charts 无原生两线间填充，且非理解指标所必需）。
- 不改变既有 STD/SMA/EMA/MACD/KDJ 语义与看板内已有配置。
- 不限制 DAY 之外周期：WEEK 下同公式表示周涨幅偏差，无需特殊处理。

## Compatibility and open questions

- HTTP 请求体新增 `kind:"RETZ"` 及 `smooth`/`regime` 字段；旧请求不受影响。看板配置读写走同一校验函数，新旧配置兼容。
- 无持久化、迁移、镜像与外部依赖变化；预期不触发 `--mysql`/`--image` 门禁，完成时按工程标准复核并记录。

## Approval scope and evidence

用户于 2026-09-21 批准本 requirements（REQ-RETZ-001 至 REQ-RETZ-005）与同目录 [design](design.md)，批准内容版本见 frontmatter `approved_revision`。批准后按工程门禁实施并回填受影响设计文档；语义修订需绑定新内容版本重新批准。验收证据见验证文档。
