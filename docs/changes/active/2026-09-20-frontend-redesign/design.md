---
id: CHG-2026-09-20-frontend-redesign-DESIGN
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-20"
approved_revision: "27398ff:docs/changes/active/2026-09-20-frontend-redesign/design.md"
approved_scope: [TWR-001, TWR-002, TWR-003, TWR-004, TWR-005, TWR-006, TWR-007, TWR-008, TWR-009, TWR-010]
---

# 目标设计：TV 风格工作台与自选清单

基线：main（27398ff）+ CHG-2026-09-20-strategy-frontend 合并后。

视觉主旨：**终端级行情驾驶舱**——近黑蓝的整幅画布上，唯一发光的 TV 蓝只留给"当前与动作"（激活 tab、聚焦、选中边条），行情数字以等宽表格数字呼吸，屏幕的每一寸最终都属于图表。结构靠留白与分隔线表达，不靠卡片盒子。

## 1. 前端

### 1.1 布局与 App 结构

`App` 外层改为两行网格：顶栏 `topbar`（48px，跨双列）+ 下方双列（自选面板 280px + 主工作区 1fr）。

```text
┌──────────────────────────────────────────────────────┐
│ ◆ Trading   [图表|扫描|回测]            [ ⌕ 搜索 ... ] │  topbar
├───────────┬──────────────────────────────────────────┤
│ 自选 (N) ⟳ │ 证券名 代码 ★   12.34  +1.20%            │
│ 浦发银行   │ 日|周  前复权|不复权  指标  v123          │
│ 12.34 +1.2%│ ┌────────────────────────────────────┐   │
│ 中信证券 … │ │        K 线主图 / 副图               │   │
└───────────┴─┴────────────────────────────────────┴───┘
```

- `view-tabs` 从 workspace-column 移入 topbar，逻辑（URL `tab` 同步、白名单回落）不变。
- 欢迎空态（无 symbol 时图表视图）保留，改用新视觉。
- 移动端（<768px）：topbar 保留品牌、tabs、搜索图标；搜索图标打开全屏浮层搜索；自选面板变为左侧抽屉（topbar 菜单按钮 + scrim，沿用既有侧栏抽屉模式）。

### 1.2 自选状态管理

`App` 持有 `watchlist: { items: WatchlistItem[]; status: 'loading' | 'ready' | 'error' }`，挂载时 `listWatchlist()` 拉取。变更函数 `toggleWatch(instrument)`：在列表中→`removeWatchlistItem`，不在→`addWatchlistItem`；两者都返回更新后的完整列表，直接替换 state，失败时保留原列表并向图表头部/面板透出错误。`WatchlistPanel` 与图表头部 ★ 均以 props 接收，不引入全局状态库。选中自选行调用既有 `selectSymbol(instrument, 'chart')`。

### 1.3 API 客户端（api/client.ts 扩展）

```ts
export interface WatchlistItem extends InstrumentSummary {
  close: number | null
  change: number | null
  change_pct: number | null
}
listWatchlist(): Promise<WatchlistItem[]>                              // GET /api/v1/watchlist
addWatchlistItem(instrument: string): Promise<WatchlistItem[]>         // POST /api/v1/watchlist
removeWatchlistItem(instrument: string): Promise<WatchlistItem[]>      // DELETE /api/v1/watchlist/:instrument（URL 编码）
```

沿用 `request<T>` Envelope 解析；dev 模式 mock 回退仅覆盖搜索与图表，自选接口不做 mock（无后端时显示错误态）。

### 1.4 WatchlistPanel（features/watchlist）

- 头部：标题"自选" + 数量徽标 + 刷新按钮（重新 `listWatchlist`，失败提示）。
- 行（高 40px）：左列名称 + 代码/交易所徽标；右列最新价 + 涨跌幅（红涨绿跌，`tabular-nums`；null 显示 —）。当前 symbol 行高亮（左侧强调色边条）。悬停显示移除 ×（调 `removeWatchlistItem`）。
- 三态：加载骨架行、错误态（含重试）、空态文案"搜索证券后点击图表头部 ★ 添加自选"。

### 1.5 顶栏搜索（InstrumentSearch 重构）

`InstrumentSearch` 重构为紧凑 combobox：输入框 + 绝对定位下拉结果（原 heading/footnote 移除），防抖 220ms、AbortController 取消、上下键/Enter/Escape 键盘导航与 combobox/listbox aria 语义全部保留（既有测试对应更新）。下拉行：名称+代码+交易所标签；输入框右侧显示 `/` 快捷键提示（kbd 徽标），全局 keydown 监听 `/`（未处于输入态时）聚焦搜索框。选中→`onSelect`（App 内进入图表视图）并清空输入。下拉入场 120ms fade + 2px 下移动画。

### 1.6 图表与面板适配

- `ChartWorkspace` 头部：证券名 + 代码 + ★ 收藏开关（props：`watched`、`onToggleWatch`），行情数值排版改等宽数字；工具栏压缩至 44px。查询世代、分页、指标逻辑不变。
- `FinancialChart`：`createChart` 配色改为新色板（背景/网格/文字/十字线/蜡烛红绿 `#ef5350`/`#26a69a`），蜡烛与成交量颜色规则（close≥open 红）不变；指标线色板保留。
- 扫描/回测面板：容器与表单控件改用新 CSS 变量（面板底色、输入框、按钮、表格），布局适配主区新 padding，功能与测试断言不变。

### 1.7 视觉规范

- 色板（CSS 变量，`styles.css` 重写，与上方交互预览一致）：`--bg:#0b0e14`、topbar `#0e1219`、panel `#10141c`、raised `#151b26`、line `#1d2432`、hover `#171e2a`、text `#d6dde8`、muted `#8792a6`、faint `#5a6478`、accent `#2962ff`（hover `#1e53e5`）、up `#ef5350`、down `#26a69a`；聚焦环 `rgba(41,98,255,.15)` 3px 外扩。
- 字体：沿用 Inter 系统栈；全部数值 `font-variant-numeric: tabular-nums`；基准 12px，行主体 13px，辅助 10–11px，图表头部报价 20px 等宽。
- 结构表达：全站仅三处结构性分隔线（topbar 下缘、自选右缘、工具栏下缘）；区域用底色层级（bg/panel/raised/hover）而非边框盒子区分；4px 间距基数（8/12/16/24）。
- 组件形态：segmented 控件改为无边框组（激活项 raised 底色 + 1px 内描边，非嵌套描边）；工具栏按钮为纯文字 + hover 底色；tab 激活为底边 2px 蓝色下划线（transform-origin 左侧滑入）；★ 使用描边星形，收藏后填充 `#f5a623`。
- 欢迎空态重设计：保留深色网格底与品牌标识，改为"按 `/` 或点击右上角搜索开始"的单一行动指引 + 三个视图说明行，删除装饰蜡烛动画块。
- 动效（CSS transition，克制）：搜索下拉 120ms fade+2px 下移；自选行 hover 100ms；tab 下划线 160ms 滑入；★ 切换 140ms 缩放脉冲；列表移除行 120ms 高度折叠。
- 焦点可见性：保留 `:focus-visible` 轮廓（accent 蓝）。
- 图表（lightweight-charts）配色与 CSS 色板同源：背景 `#0b0e14`、网格 `#161c28`、文字 `#8792a6`、蜡烛红 `#ef5350` / 绿 `#26a69a`、成交量按涨跌着色 0.5 透明度、十字线 `#7c8ba1`。

## 2. 后端

### 2.1 表与迁移

`WatchlistModel`（`t_watchlist`）：`BaseModel` + `Exchange`（uniqueIndex `uq_watchlist` p1）+ `Code`（p2），无用户列。`migrationModels` 追加 `{&WatchlistModel{}, []string{"uq_watchlist"}}`，受控 AutoMigrate 建表（新增表，符合运行手册）。排序即插入顺序（id 升序）；上限 100 由应用层校验。

### 2.2 端口（port/watchlist.go）

```go
type WatchlistEntry struct {
    port.InstrumentSummary          // 完整身份与名称等
    InstrumentRowID uint64          // t_instruments 主键，供报价查询
}
type WatchlistStore interface {
    List(ctx) ([]WatchlistEntry, error)            // join t_instruments，仅活跃，按添加序
    Add(ctx, market.InstrumentID) (exists bool, err error)   // 幂等
    Remove(ctx, market.InstrumentID) error                     // 不存在也成功
    Count(ctx) (int, error)
}
type DailyQuote struct { Close, Change, ChangePercent float64 }
type DailyQuoteReader interface {
    LatestDailyQuotes(ctx, ids []uint64) (map[uint64]DailyQuote, error)
}
```

### 2.3 应用服务（application/watchlist_service.go）

`WatchlistService{store, catalog}`：

- `List`：store.List → DailyQuoteReader 批量报价 → 组装 DTO；无报价条目 close/change/change_pct 为 null。
- `Add`：`InstrumentID.Validate` → 400；catalog.Get 校验存在且活跃 → 404；`Count>=100` → 409（上限已满）；store.Add 幂等；返回更新列表。
- `Remove`：Validate → 400；store.Remove 幂等；返回更新列表。

错误沿用应用层既有错误类型与映射约定。

### 2.4 MySQL 仓储（infrastructure/mysql/watchlist_store.go + 报价查询）

- `List`：`t_watchlist` JOIN `t_instruments`（exchange+code 相等且 active），按 `t_watchlist.id` 升序。
- `Add`：`INSERT ... ON DUPLICATE KEY UPDATE id=id` 风格幂等（GORM `clause.OnConflict{DoNothing}`），事务内先查活跃。
- 报价：单条窗口函数 SQL，取每只证券最新两根当前日线 bar（`timeframe='DAY' AND valid_to_version IS NULL`，`ROW_NUMBER() OVER (PARTITION BY instrument_id ORDER BY close_time DESC)`，rn≤2），按 `ValueScale`（10000）换算为元；`change=close-prev_close`，`change_pct=change/prev_close*100`（保留两位由前端处理，后端传原值）。空集合跳过查询。

### 2.5 API（每接口独立文件）与路由

- `api/get_watchlist.go`：`GET /api/v1/watchlist` → 200 `{items:[...]}`。
- `api/add_watchlist_item.go`：`POST /api/v1/watchlist`，严格 JSON DTO（拒绝未知字段、1MiB 上限，复用既有解析助手）→ 200 `{items:[...]}`；错误映射 400/404/409。
- `api/remove_watchlist_item.go`：`DELETE /api/v1/watchlist/:instrument`（客户端对 `SSE:600000` 整段 URL 编码）→ 200 `{items:[...]}`；身份非法 400。
- `KernelServices` 增加 `Watchlist application.WatchlistService`；`router.go` 注册三条路由。
- 响应项：`instrument/code/name/exchange/board/lot_size/close/change/change_pct`（后三者可 null）。

## 3. 文档同步

`web.md`（owns 增 `features/watchlist/`、布局与视觉规范）、`api.md`（自选能力）、`standards/http-api.md`（三接口）、`internal/infrastructure/mysql.md`（t_watchlist）、`internal/port.md`、`internal/application.md`、`design/README.md` 模块地图与源码目录、本变更记录。

## 4. 测试策略

- Go：handler 测试（严格 JSON、400/404/409、幂等、响应结构）；service 测试（校验流、报价组装、null 语义）；dbtest 远端随机库集成（建表迁移、CRUD、窗口函数报价、非活跃过滤、上限）。不使用 SQL mock 验收。
- 前端：`client.test.ts`（三端点 URL/method/body/编码）；`WatchlistPanel.test.tsx`（渲染/报价着色/点击/移除/高亮/三态/刷新）；`InstrumentSearch.test.tsx` 更新（紧凑形态、键盘/防抖/取消保留）；`App.test.tsx`（布局结构、★ toggle 流、tab 移位后路由语义不变）；`ChartWorkspace.test.tsx` 适配（★ props）。

## 5. 实施顺序（每步可编译、测试全绿）

1. 后端：model+迁移+端口+仓储+服务+handler+路由（含测试）。
2. 前端基础：client watchlist 函数 + App 布局重构（topbar/tabs/搜索迁移）+ styles.css 重写（含测试更新）。
3. WatchlistPanel + 图表 ★（含测试）。
4. 图表配色/密度细化 + 扫描/回测面板适配 + 移动端断点（含测试与走查）。
5. 文档同步 + 变更记录验收 + 独立子 Agent 评审。

## 6. 回滚

后端：`t_watchlist` 为新增独立表，回滚即服务回退版本并 `DROP TABLE t_watchlist`（无外部引用）。前端：恢复前端文件即可；无既有接口语义变化。
