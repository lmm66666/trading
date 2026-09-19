---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["web/", "web/src/", "web/src/api/", "web/src/features/chart/", "web/src/features/indicators/", "web/src/features/search/", "web/src/features/strategy/", "web/src/features/scan/", "web/src/features/backtest/", "web/src/test/"]
related: []
---

# 行情工作台前端设计

想先理解图表页的数据从哪里来、怎样翻页，请读 [图表查询](workflows/chart-query.md)。本文保留前端状态、组件与 API 客户端契约。

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `web` |
| 最后更新 | 2026-09-20 |

## 1. 职责与非职责

本模块是 React 行情工作台，负责证券搜索、日/周和 RAW/QFQ 切换、K 线与成交量展示、指标选择、固定版本向前分页、URL 状态和用户可理解的加载/错误反馈；并提供策略扫描与回测两个视图：策略目录选择与参数输入、扫描/回测任务的创建与轮询跟踪、扫描入选名单与失败明细分页、回测摘要/权益曲线/订单成交展示。

前端不计算服务端技术指标或策略信号、不推断证券交易所、不选择不同页的最新行情版本、不保存历史任务列表（每类视图仅恢复最近一个任务标识）。

## 2. 组件和边界

- `App` 管理已选证券、周期、复权视图、视图切换（图表/扫描/回测，URL `tab` 白名单）和移动搜索开关，并同步 URL；持有扫描/回测任务标识（localStorage `wb.scan_run_id`、`wb.backtest_run_id`）。
- `InstrumentSearch` 对输入做 220ms 防抖，支持键盘导航，取消过期请求并返回完整证券身份。
- `ChartWorkspace` 管理查询世代、固定版本分页、指标列表和页面状态。
- `FinancialChart` 把 Bar 和指标 Series 映射到主图/副图。
- `IndicatorManager` 只管理服务端支持的参数预设，不在浏览器重新计算指标。
- `features/strategy` 提供共享的策略表单（目录驱动的参数输入与 min/max/integer 校验）、任务轮询 hook（2 秒固定间隔、终态停止、连续 3 次网络错误停止）和任务状态条（状态徽标、取消入口）。
- `features/scan` 提供扫描表单（时间窗口、交易所范围、active_only、limit）与入选名单/失败明细分页（游标 `after_sequence`）。
- `features/backtest` 提供回测表单（执行假设 config、元→缩放整数换算）与结果区（summary 指标卡、权益曲线、订单/成交分页表）。
- `api/client` 定义前端契约 DTO、统一解析响应 Envelope 并转成人类可读错误；集中承载金额缩放换算（×10000）纯函数。

后端契约见 [API 文档](../standards/http-api.md)。

## 3. 核心状态与不变量

- URL 只接受合法的完整 A 股身份、DAY/WEEK 和 RAW/QFQ；非法值回落到安全默认值。
- 选择证券或改变周期/复权视图时创建新的查询世代，清空旧结果并取消首屏/分页请求。
- 首次图表请求使用服务端解析的正 `data_version`；加载更早数据必须传回同一版本和 `next_before`。
- 旧页只有证券、周期、价格视图和版本都与当前结果一致时才允许合并。
- Bar 按 close_time 去重并升序；指标按稳定 key 合并，点按 time 去重并升序。
- 已取消或旧世代响应不得修改 UI，即使网络层稍后完成。
- loading、ready、error 和“空行情”是不同状态；分页失败保留已经展示的数据并显示提示。
- 任务轮询到达终态（含 PARTIAL_SUCCEEDED）即停止；连续 3 次网络错误停止轮询；面板卸载即停，任务在后端继续。
- 扫描结果定位使用终态响应的 `snapshot_id` 与任务回显的策略/版本，不用“仅按策略查 latest”的模糊路径；结果分页固定首响应的快照 key 续读。
- 金额与价格在 UI 输入以“元”表达，提交前集中换算为缩放 10000 的整数；展示时反向换算；大整数边界由表单校验（初始资金 ≤1e9 元）保证 Number 精度。

## 4. 主要流程

```text
搜索并选择完整证券身份
  → 请求固定版本图表页
  → 展示 K 线、成交量和指标
  → 接近历史边界或点击加载
  → 使用同 data_version + next_before 请求旧页
  → 去重合并
```

指标变更会重新发起完整首屏查询，避免把不同指标集合的分页结果混合。

## 5. 交互和可访问性

- 搜索输入使用 combobox/listbox 语义，支持上下键、Enter 和 Escape。
- 周期和价格视图按钮暴露 `aria-pressed`，弹层和关闭按钮提供明确标签。
- 移动端搜索使用侧栏、遮罩和可操作关闭入口。
- 红涨绿跌是当前产品约定；文字和数值同时表达变化，不只依赖颜色。

## 6. 失败与安全语义

- 非 JSON 或非成功 Envelope 映射为简洁错误，不渲染服务端内部详情。
- 搜索错误清空候选，图表首屏错误进入错误态；分页错误不删除当前结果。
- query 参数在进入状态前白名单校验，不能直接作为 HTML 或请求路径片段。
- 浏览器不保存凭据和数据库信息。

## 7. 性能约束

- 首屏与分页默认 400 根 Bar，指标请求由后端计算预算控制。
- 同类分页同一时间最多一个，新的分页取消旧分页。
- 图表数据合并保持确定排序，避免重复点导致图形抖动。

## 8. 测试与验收证据

Vitest 与 Testing Library 覆盖 URL 状态、搜索防抖/键盘/取消、查询世代、版本固定分页、合并去重、指标管理、图表交互和错误状态；以及策略目录渲染与参数校验、任务提交体与幂等键、轮询节奏/终态停止/取消、扫描快照游标分页与失败明细、回测换算与摘要渲染、权益曲线连续拉取、订单/成交分页。

```bash
npm --prefix web run check
```

覆盖率门禁由前端脚本执行。

## 9. 相关文档

- [API 设计](api.md)
- [HTTP 契约](../standards/http-api.md)
- [系统设计](../architecture/system-design.md)
