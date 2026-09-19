---
id: CHG-2026-09-20-strategy-frontend
status: draft
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-20"
approved_revision: "c69c9fa:docs/changes/active/2026-09-20-strategy-frontend/"
approved_scope: [SWF-001, SWF-002, SWF-003, SWF-004, SWF-005, SWF-006, SWF-007, SWF-008, SWF-009, SWF-010]
---

# 前端策略功能：扫描与回测

| 属性 | 内容 |
|---|---|
| 创建日期 | 2026-09-20 |
| 适用范围 | `web`（`api/client`、新增 `features/strategy`、`features/scan`、`features/backtest`、`App`、`chartData` URL 状态）、`docs/design/web.md` 与 `docs/design/README.md` 文档同步 |
| 基线 | 当前 main（工作树） |
| 已确认意图 | 用户于 2026-09-20 会话裁决：本期只做"把策略功能（扫描 + 回测）做上前端"，后端 API 已齐备；TV 仅作视觉参考，不做绘图工具、不做自选清单、不做整体视觉改版 |
| 批准证据 | 用户于 2026-09-20 会话通过评审界面批准本需求全量（SWF-001–SWF-010） |

## 1. 问题、目标与使用条件

后端已提供策略目录、扫描任务、信号快照和回测任务的完整 HTTP 契约（见 [HTTP 契约](../../../standards/http-api.md)），但前端行情工作台只有证券搜索与图表查询，用户无法在浏览器里使用策略扫描和回测。本变更在现有 React 工作台内新增扫描与回测两个视图，复用现有视觉语言与状态管理模式，不修改任何后端行为。

## 2. 稳定需求

- SWF-001 策略目录：用户可查看服务端策略目录（`GET /api/v1/strategies`），包括策略、版本、主周期、预热根数、默认持有期和参数定义（default/min/max/integer）；目录加载失败显示可读错误。
- SWF-002 发起扫描：用户选择策略与版本、可选参数、时间窗口 `from`/`as_of`（日期输入，转 UTC RFC3339）和范围（`exchanges` 多选、空表示全部；`active_only`；`limit` 1–5000）后创建扫描任务；幂等键由前端生成 UUID；提交后展示 `run_id` 与状态。
- SWF-003 任务状态跟踪：扫描与回测创建后以固定间隔轮询 `GET .../runs/:run_id`，展示 `status`、`data_version`、`attempts` 与错误 message；运行中可显式取消；切换视图或卸载面板后轮询停止，任务在后端继续；PENDING/RUNNING 之外的终态停止轮询。
- SWF-004 扫描结果：任务 `SUCCEEDED`/`PARTIAL_SUCCEEDED` 后按保存的快照 key 分页读取入选行（完整 instrument、名称、`signal_time` 本地时区展示），满页以 `next_sequence` 作为 `after_sequence` 续读，无 `next_sequence` 即结束；`PARTIAL_SUCCEEDED` 同时可折叠展示 `failures`（instrument/code/message/retryable）；点击入选证券跳转到该证券的图表视图。
- SWF-005 发起回测：用户选择证券（默认带入当前选中证券，可用现有证券搜索更换）、策略与版本及可选参数、时间窗 `start`/`end`、执行假设 config（初始资金、现金使用比例、佣金、最低佣金、印花税、过户费、滑点、手数、持有期）后创建回测任务；金额以"元"输入，前端换算为缩放 10000 的整数；`lot_size` 默认取当前证券 `lot_size`，`hold_bars` 留空表示使用策略默认。
- SWF-006 回测结果：任务 `SUCCEEDED` 后展示 summary（总收益、年化收益、最大回撤、胜率、盈利因子、平均持有根数、平仓笔数、期末持仓标记）、权益曲线图与订单、成交列表；列表以 `after_sequence` 游标分页；缩放整数金额展示换算为元，比例展示为百分比；summary 中无定义的比例按空值展示。
- SWF-007 视图组织：工作台提供 图表 / 扫描 / 回测 三个视图切换，URL 参数白名单同步、非法值回落图表视图；新面板沿用现有深色视觉语言（任务状态条、指标卡、表格），不引入新主题与新布局框架。
- SWF-008 数值边界：初始资金输入限 1 千–10 亿元（缩放后 ≤1e13，JavaScript Number 精度安全）；费率与比例 0–10000bps；日期范围不超过 20 年；表单提交前校验，非法值不发出请求。
- SWF-009 会话内任务恢复：当前扫描与回测的 `run_id` 保存到 localStorage，刷新或重开后可恢复查看该任务状态与结果；不提供历史任务列表（后端无枚举接口）。
- SWF-010 错误语义：复用统一 Envelope 解析与可读错误；`IDEMPOTENCY_CONFLICT` 提示重新提交（自动换新幂等键）；其余错误在对应面板进入错误态并保留已展示内容。

## 3. 非目标

- 不修改后端 API、持久化、策略内核与 Worker 行为；不新增任何接口。
- 不做历史任务列表、多任务并行管理面板；不做 WebSocket/推送，轮询即可。
- 不做绘图工具、自选清单、整体视觉改版、明暗主题切换。
- 不做策略参数预设的保存与命名。
- 不在浏览器计算指标或信号；参数白名单完全由服务端策略目录驱动。

## 4. 方案比较与选择

选择：在现有工作台内加视图切换（图表/扫描/回测），共享上下文。

放弃独立多页路由（扫描、回测做成独立页面）：会丢失"扫描结果点击 → 看图"和"回测默认当前证券"的上下文联动，且现有 URL 状态模式只需扩展一个白名单字段。放弃右侧抽屉面板：扫描表单与结果表格体量大，抽屉空间不足。权益曲线选择 Lightweight Charts 折线（复用现有图表依赖），不引入新图表库。

## 5. 可执行验收

| 需求 | 验证 |
|---|---|
| SWF-001 | Vitest：目录加载渲染参数定义与默认值；加载失败错误态 |
| SWF-002 | Vitest：提交体字段断言（含 UUID、日期转 UTC、scope 默认值） |
| SWF-003 | Vitest（fake timers）：轮询节奏、终态停止、取消调用、卸载停止、连续网络错误停止 |
| SWF-004 | Vitest：首屏 key 固定、`after_sequence` 续页、`next_sequence` 缺失结束、failures 折叠、行点击回调 |
| SWF-005 | Vitest：元→缩放整数换算、`lot_size`/`hold_bars` 默认、参数 min/max 校验拦截提交 |
| SWF-006 | Vitest：summary 渲染（含空值）、金额 ÷10000 展示、equity 连续拉取至无 `next_sequence`、订单分页 |
| SWF-007 | Vitest：`tab` 参数白名单与非法值回落 |
| SWF-008 | Vitest：越界输入不出请求 |
| SWF-009 | Vitest：localStorage 写入与恢复 |
| SWF-010 | Vitest：409 提示与错误态保留内容 |
| 门禁 | `npm --prefix web run check`（含覆盖率门禁）；Go 侧无改动，全局门禁 `bash scripts/verify.sh` 照常执行 |

## 6. 风险

- 全市场扫描（5000 只）耗时可能达分钟级，轮询期间用户切走视图后任务继续；`run_id` 持久化到 localStorage 保证刷新后可恢复查看，属可接受的使用体验而非缺陷。
- 契约要求大整数客户端无损处理；本设计以 Number 承载并把输入限制在 1e9 元内（缩放 ≤1e13，远小于 2^53），该边界写入 SWF-008 由表单校验保证。若未来需要更大资金规模，须另行引入 BigInt 解析。
- 权益曲线对 20 年日线（约 5000 点）需连续拉取多页，首次展示可能多次请求；可接受且不阻塞 summary 先行展示。
