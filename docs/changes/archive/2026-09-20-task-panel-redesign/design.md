---
id: CHG-2026-09-20-task-panel-redesign-DESIGN
approval_status: approved
authority: proposed
approved_by: 用户（对话）
approved_at: "2026-09-20"
approved_revision: sha256:0f2262608ae136154b0b5ffacbac1fa48e1d56abc1140db920dd8ca1e03314ad
approved_scope:
  - docs/changes/active/2026-09-20-task-panel-redesign/requirements.md
  - docs/changes/active/2026-09-20-task-panel-redesign/design.md
---

# 扫描/回测任务面板重设计 design

需求与验收标准见 [requirements.md](requirements.md)。本设计对应已批准的高保真原型（`/tmp/trading-taskpanel-proto/`）。

## Current and target behavior

当前：扫描/回测表单平铺在单张卡片内（`ScanPanel`/`BacktestPanel` 的 `panel-card`），时间为两个原生 `<input type="date">`，交易所为原生 checkbox，回测 9 个执行假设字段全部平铺；结果区（RunMonitor + 结果）跟在表单下方，宽屏下右侧大面积空置。

目标：每个任务视图改为 `task-workspace` 双栏网格——左 `config-column`（380px，独立滚动，承载分区表单卡片），右 `result-column`（任务状态条置顶、空态引导、结果卡片）；<900px 单栏堆叠。表单按「策略 / 时间范围 / 范围（扫描）/ 执行假设（回测）」分区，日期由共享 `RangePicker` 组件承担。

## Data sources, model, and flow

- 无新增数据源；全部状态仍为组件内 `useState`。`RangePicker` 受控：`{ from: string; to: string }`（`YYYY-MM-DD` 本地日期字符串），通过 `onChange(from, to)` 回写面板状态，仅在点击「确定」（完整区间）或「清除」（空区间）时回写，弹层内中间态不污染表单。
- 日期字符串继续经 `toRFC3339` 转 UTC 零点提交；`validateDateRange` 原样复用。
- 预设计算基于本地日期：`近 N 月` = 今日减去 N 个自然月，`今年以来` = 当年 1 月 1 日，终点均为今日。
- 日历渲染：以「当前月的前一月」为初始左月，双月并列；周一起始；未来日期（>今日）禁用。

## Interface and contract changes

- 新增 `web/src/features/strategy/RangePicker.tsx`（归属既有 `features/strategy/` 目录，无新增 owns）：
  - Props: `from: string`、`to: string`、`onChange: (from: string, to: string) => void`、可选 `ariaLabel?: string`。
  - 触发按钮 `aria-haspopup="dialog"` + `aria-expanded`；弹层 `role="dialog"`；月份容器 `role="group"` + `aria-label="YYYY 年 M 月"` 供测试与辅助技术区分双月；日单元格为按钮，`disabled` 表示未来日期；起点/终点/区间内单元格以类名与 `aria-pressed` 表达。
- `ScanPanel`：时间字段由两个 label+input 换成 `RangePicker`（`ariaLabel="扫描时间范围"`）；交易所 checkbox 换成 `aria-pressed` chips；`active_only` 换为开关样式的 checkbox（label 不变）。
- `BacktestPanel`：时间字段同样换 `RangePicker`（`ariaLabel="回测时间范围"`）；佣金/最低佣金/印花税/过户费/滑点五项移入「高级费用设置」折叠区（按钮 `aria-expanded`，折叠条含当前费用摘要）；初始资金、现金使用比例、每手股数、持有期保持可见。
- `StrategyForm`：参数输入列表包一层 `param-grid` 两列网格容器；校验与 `onChange` 语义不变。
- `RunMonitor` 组件不变，仅由父级移动到右栏顶部。
- HTTP 契约、DTO、localStorage 键、URL 状态：无变化。

## Errors, degradation, and recovery

- 未选日期提交：`validateDateRange` 返回"请选择开始与截止日期"，不发请求（现有语义）。
- 弹层打开中卸载面板或切换视图：组件随卸载销毁，无悬挂监听器（outside-click/Escape 监听在关闭或卸载时移除）。
- 窄屏：<900px 弹层单月显示（第二月历 CSS 隐藏，左月历提供双向导航），表单单栏堆叠。

## Concurrency, capacity, performance, and security

- 纯客户端组件；日历每次渲染最多 2×42 个单元格，无性能敏感路径；不引入新依赖。
- 日期字符串仅用于提交体转换与显示，不插入 HTML；无新安全边界。

## Compatibility, migration, and rollback

- 无迁移；提交体与校验规则不变，服务端无感知。回滚 = 还原本变更全部文件，无残留状态。

## Alternatives and tradeoffs

- 原型中的运行进度条不实现：后端 `RunStatus` 无进度字段，伪造进度会误导用户；保持现有状态徽标语义。
- 不引入第三方日期库（react-daypicker 等）：需求（双月、预设、区间预览）可用约 200 行自研组件覆盖，避免新依赖与主题适配成本。
- 「确定/清除」显式提交区间（而非点选即回写）：避免半选状态进入表单校验，行为可预测；代价是多一次点击，已由原型评审接受。

## Test strategy

- `RangePicker.test.tsx`（新增）：打开/关闭（触发、Escape、外部点击）、预设回写正确区间、日历正/反向点选与交换、悬停预览类名、未来日期禁用、清除回写空区间、未选完整区间确定禁用。
- `ScanPanel.test.tsx`（更新）：时间输入改经 RangePicker（固定系统时间后点预设）；chips 的 `aria-pressed` 与提交体；空态展示；其余任务生命周期用例保持不变。
- `BacktestPanel.test.tsx`（更新）：同上日期交互；高级费用默认折叠/展开后可编辑；其余保持不变。
- 门禁：`npm --prefix web run check`、`go test ./...`、`go vet ./...`、`bash scripts/verify.sh`（无 SQL/Docker 变化，`--mysql`/`--image` 记 not-required）。

## Affected current designs, standards, and ADRs

- 完成后回填 [前端设计](../../../design/web.md) 第 1、2、5、8 节：双栏任务工作区、RangePicker 组件、chips/开关/折叠控件、空态与新测试覆盖描述。owns 不变（新组件落在 `web/src/features/strategy/`）。
- 不影响架构、HTTP 契约与其他标准；无 ADR。

## Approval scope and evidence

批准记录同 [requirements.md](requirements.md#approval-scope-and-evidence)：用户于 2026-09-20 评审交互原型并指示执行；本设计为原型文字化，`approved_revision` 为两文档正文（不含 frontmatter）的 sha256 摘要。
