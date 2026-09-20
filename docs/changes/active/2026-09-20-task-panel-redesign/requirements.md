---
id: CHG-2026-09-20-task-panel-redesign
status: approved
approval_status: approved
authority: proposed
approved_by: 用户（对话）
approved_at: "2026-09-20"
approved_revision: sha256:0f2262608ae136154b0b5ffacbac1fa48e1d56abc1140db920dd8ca1e03314ad
approved_scope:
  - docs/changes/active/2026-09-20-task-panel-redesign/requirements.md
  - docs/changes/active/2026-09-20-task-panel-redesign/design.md
---

# 扫描/回测任务面板重设计 requirements

## Problem and user scenarios

用户在使用扫描与回测视图时，表单平铺在单一卡片内、右侧大片区域空置，时间窗口使用原生 `<input type="date">`，深色主题下占位符与日历图标不可控、无可视化区间选择体验。用户（工作台使用者）需要：紧凑有序的配置区、图形化日期范围选择、结果区有明确空态引导。

场景：用户打开扫描视图 → 选择策略与参数 → 通过日历弹层选择扫描时间窗口（或点快捷预设）→ 发起任务 → 在右侧查看状态与入选名单；回测视图同理，费用参数默认折叠。

## Environment, frequency, and data volume

- 环境：浏览器端 React 工作台（`web/`），桌面为主，移动端（<768px）可用性不退化。
- 频率：每用户每天发起扫描/回测数次至数十次；表单交互为纯客户端行为，无新增服务端负载。
- 数据量：日期选择覆盖近 20 年区间（既有校验上限），日历一次渲染两个月（约 62 个单元格），无分页或大数据集。

## Requirements

- `REQ-TPR-001`: 扫描与回测视图采用左右双栏工作区：左侧固定宽度配置面板（独立滚动），右侧为任务状态条与结果区；窄屏（<900px）退化为上下堆叠。
- `REQ-TPR-002`: 提供共享的日期范围选择器组件，替代原生 `date` 输入：触发框展示已选区间，弹层包含快捷预设（近 1 月/3 月/6 月/1 年/3 年、今年以来）与双月历；点击起点后悬停预览区间，点击终点完成选择，反向选取自动交换；未来日期禁用；确定后回写 `YYYY-MM-DD` 字符串，支持清除；Escape 或点击外部关闭。
- `REQ-TPR-003`: 表单控件重组：交易所多选改为 `aria-pressed` toggle chips；"仅活跃证券"改为开关样式（语义仍为 checkbox）；回测的五项费用参数收进默认折叠的"高级费用设置"，折叠条摘要当前费用值；策略参数以两列网格排布；提交按钮通宽置于表单底部。
- `REQ-TPR-004`: 无任务运行时结果区展示居中空态引导；任务状态条移至右侧结果区顶部，组件语义不变。
- `REQ-TPR-005`: 提交契约不变：扫描/回测请求体、`toRFC3339` 转换、`validateDateRange` 校验规则（两端必填、顺序、20 年跨度上限）、幂等键与轮询行为保持现有语义。

## Acceptance criteria

- 扫描/回测视图在桌面宽度下呈现左配置、右结果双栏；窗口收窄至 <900px 时单栏堆叠且表单可用。
- 日期范围选择器：点预设"近 1 月"并确定后提交体 `from`/`as_of`（或 `start`/`end`）为对应 `YYYY-MM-DD`；日历反向点选自动交换；不选日期提交显示"请选择开始与截止日期"且不发出请求。
- 交易所 chips 选中态 `aria-pressed="true"` 并进入提交体 `scope.exchanges`；全部不选中提交为空数组。
- 回测"高级费用设置"默认折叠、费用输入不可见；展开后可编辑并随提交体上送。
- 既有任务生命周期行为（提交、轮询、取消、幂等冲突提示、结果分页）回归测试全部通过。
- `npm --prefix web run check` 通过，前端覆盖率不低于现有门禁。

## Scope and non-goals

- 范围：`web/src/features/scan/`、`web/src/features/backtest/`、`web/src/features/strategy/` 内的表单与布局组件，以及 `web/src/styles.css`。
- 非目标：不改变任何 HTTP API、请求体字段或服务端行为；不重做图表视图与自选面板；不引入第三方日期/组件库；不实现运行进度条（后端 `RunStatus` 无进度数据，原型中的进度条为演示元素，不落地）；不保存多套表单草稿。

## Compatibility and open questions

- 兼容：提交契约、URL 状态、localStorage 任务标识键不变；`validateDateRange`/`toRFC3339` 继续以 `YYYY-MM-DD` 字符串工作。
- 无未决问题；布局断点 900px 为设计决策，已通过原型评审。

## Approval scope and evidence

批准记录：用户于 2026-09-20 评审高保真交互原型（`/tmp/trading-taskpanel-proto/`，覆盖扫描与回测两个视图的完整交互）后明确指示"执行吧"。本文件与 [design.md](design.md) 为已批准原型的文字化，布局、组件与交互与原型一一对应；与原型的唯一偏差（不实现进度条）已在设计"Alternatives and tradeoffs"声明并属批准范围。`approved_revision` 为两文档正文（不含 frontmatter）的 sha256 摘要，批准范围即上述两文档全部内容。

This file alone owns the change lifecycle `status`. Link verification results instead of duplicating them.
