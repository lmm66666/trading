---
result: passed
---

# TV 风格工作台与自选清单验证记录

| 属性 | 内容 |
|---|---|
| 验证日期 | 2026-09-20 |
| 验证范围 | TWR-001 ~ TWR-011 全部稳定需求 |
| 实施提交 | 827e235（后端自选全链路）、前端各步提交至 e0321be（TWR-011 收尾） |

## 1. 需求验收（对照 requirements.md §5）

| 需求 | 结果 | 真实验证证据 |
|---|---|---|
| TWR-001 | 通过 | `watchlist_store_integration_test.go`（远端 MySQL 随机隔离库）：建表、插入、按添加序返回、非活跃过滤；`get_watchlist_test.go`：响应结构与排序 |
| TWR-002 | 通过 | `add_watchlist_item_test.go`：严格 JSON、非法身份 400、未知/非活跃 404、重复添加幂等、上限 409 |
| TWR-003 | 通过 | `remove_watchlist_item_test.go`：URL 编码身份、不存在幂等、返回更新列表 |
| TWR-004 | 通过 | dbtest：两根 bar 计算 change/change_pct、单根/零根 null、×10000 缩放换算为元；`watchlist_service_test.go`：报价组装 |
| TWR-005 | 通过 | `App.test.tsx`（15 用例）：三段布局渲染、tab 白名单与非法值回落 |
| TWR-006 | 通过 | `InstrumentSearch.test.tsx`（5 用例）：防抖/键盘导航/取消保留、`/` 快捷键聚焦、选中进入图表 |
| TWR-007 | 通过 | `WatchlistPanel.test.tsx`（6 用例，行覆盖 100%）：行点击、移除调用、当前高亮、空/载/错三态、刷新按钮 |
| TWR-008 | 通过 | `App.test.tsx`：★ 状态反映列表、toggle 分别调用 POST/DELETE、失败提示与列表保留 |
| TWR-009 | 通过（代码断言） | Vitest：配色变量应用、`FinancialChart`/`EquityChart` 图表配色同步断言、扫描/回测面板适配渲染 |
| TWR-010 | 通过（代码断言） | CSS 断点（`@media (max-width: 767px)`）搜索全屏浮层与自选抽屉结构由 App/搜索测试覆盖 |
| TWR-011 | 通过 | `FinancialChart.test.tsx`：`computeBarSpacingLimits` 四组宽度换算（1200→{3,80}、760→{4.75,76}、320→{3,32}、1920→{4.8,128}）、ResizeObserver 触发 `applyOptions`；`ChartWorkspace.test.tsx`：自动加载触发、进行中防重复、`has_more=false` 不触发；prepend 视口平移既有用例保持 |

TWR-009/010/011 的"人工走查"部分未执行浏览器操作（按 AGENTS 约定非必要不调用浏览器）；视觉与交互手感确认留给用户在合并前走查，不阻塞代码层验收。

## 2. 门禁执行记录

| 门禁 | 结果 | 摘要 |
|---|---|---|
| `npm --prefix web run check` | 通过 | 14 个测试文件 103 用例全过；总覆盖率 90.67%（语句）/93.3%（行），watchlist 100%；tsc 与 vite build 通过 |
| `go test ./...` | 通过 | 全部包 ok（api、application、mysql、port、strategy、backtest 等 14 个有测试包） |
| `go vet ./...` | 通过 | 无告警 |
| `bash scripts/verify.sh --mysql` | 通过 | 10 步全绿：market 94.3% / indicator 91.3% / strategy 94.8% / backtest 90.4% 核心覆盖率、race detector、全市场扫描性能门禁（full_scan=201.6ms）、远端 MySQL 8.4/x86_64 随机隔离集成测试（含 t_watchlist 建表、CRUD、窗口函数报价）通过 |

未选择的外部门禁（`--image`、`--full`）按 verify.sh 输出记录为"未选择，不计为通过"；本变更不涉及镜像构建与部署。

## 3. 遗留说明

- 设计 §1.7 中"列表移除行 120ms 高度折叠"退出动画未实现（React 卸载无退出动画），属可选视觉打磨，不在验收范围。
- 自选清单为空态时报价为 null 的显示（—）已由测试覆盖；首次使用需用户手动添加。
