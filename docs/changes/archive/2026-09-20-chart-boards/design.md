---
id: CHG-2026-09-20-chart-boards-DESIGN
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-20"
approved_revision: "sha256:83466807eb8f36fc61044cd451540f271c037554a35171d71b0290a4d97685a5"
approved_scope: [BOARD-001, BOARD-002, BOARD-003, BOARD-004, BOARD-005, BOARD-006, BOARD-007]
---

# 看板目标设计

## 状态和存储（BOARD-005/006）
App 持有看板控制器以跨页签保留草稿；ChartWorkspace 消费配置，去掉按股票 key 重挂载。localStorage 单个 wb.boards.v1 对象保存 schemaVersion、activeId、boards。每看板有唯一 id、name、config（defaultSymbol、timeframe、priceView、indicators、comparison、paneWeights、visibleBars）。读取时校验结构及容量；损坏明确提示并禁止无意覆盖。写入成功后才更新已保存快照；手动保存/创建/删除/重命名为显式持久化动作。切换活动 ID 仅记住选择，不保存草稿。无记录时使用未落盘默认配置。最后一个看板不可删除，删除需确认。

## 查询与显示（BOARD-002/003/004）
主图继续 POST chart-queries 固定版本分页；关联行情 GET market/bars 使用相同正版本、daily/weekly、raw 和主图已加载日期范围，limit=5000。既有期货连续品种以明确下拉项选择，不假设目录搜索支持期货。关联失败只降级关联曲线并提供重试，不阻断股票。请求取消和世代守卫防陈旧结果覆盖。
百分比显示是前端展示变换，不改技术指标公式。首次主图窗口和关联行情的最早共同有效日期作为基准（无叠加则主图首日），分母须正且有限；OHLC 与 MA 使用主标的同一分母，关联收盘价使用该日关联分母。日期取 API close_time 的 YYYY-MM-DD（日线）；周线按 ISO 周匹配，在主图时间点显示。缺失日期使用 whitespace，既不前填也不零填。分页后复用固定基准；切标的/周期/复权/关联时重设。无共同基准提示，主图仍可单独展示。图例显示基准日期和原始价格，百分比坐标只应用于主图。
成交量独立第1副图，其余指标以完整参数身份分组，避免两个 MACD 共用同一分区。分区权重和观察根数在保存时从当前图表读取，拖动或缩放标记草稿；平移位置不保存。主图自动缩放。

## 指标（BOARD-007）
indicator 新增 STDKind、引用验证/稳定 key/构建计算；应用查询新增 STD 请求，period 1–500、其余参数为0，成本按 period 计入2000预算；按完整历史计算总体标准差再截页，支持 RAW/QFQ。计算采用窗口内两遍均值/平方差，复杂度 O(N*period) 有预算上限，避免差平方消减误差。不影响已有策略引用。
IndicatorManager 支持原有预设及 STD20，并提供已添加实例的参数编辑和移除；指标最多16，周期范围与服务端一致，错误就地显示。

## 列表（BOARD-001）
WatchlistPanel 提供本地筛选、用户触发涨跌排序和服务端顺序恢复；排序以操作时快照固定，刷新报价不重排。App 管理收起与宽度，桌面260px起、220–360px，移动沿用抽屉。增加搜索入口复用顶栏搜索，不新增 API。

## 失败、安全、兼容
localStorage 无权限/满额提示并保留草稿。主图请求与叠加请求分离，统一 AbortSignal；切换清除旧图避免错配。JSON 只保存配置、不保存行情或凭据；React 文本渲染，不插入 HTML。持久化配置带 schemaVersion，不悄悄迁移未知版本。既有 URL 作为显式访问入口，参数只读安全白名单。相同来源多页同时编辑通过保存前对比存储快照拒绝覆盖并提示重新载入。

## 影响与验证
影响 web、indicator、application 图表查询、HTTP STD 契约；技术归属仍为现有模块。现有自选/市场行情 API 不变，不新增 DB 表或索引。测试先验证手动保存/失败/恢复/切股和百分比/日期缺口，再实现；Go 测试验证总体标准差、预热、常量、前缀不变和分页。运行 scripts/verify.sh；无数据库和构建依赖变化，MySQL/image门禁不适用。完成后独立子 agent review 并合并 main，不自动推送。

## 批准依据
复用本会话用户“别自动保存，手动保存”“油价看板，可以切换不同的石油股票”“纵轴…用比例”“同图叠加一个指标”“总结一下方案，准备执行吧”及随后提供仓库路径的执行授权。容量、本地存储、国内原油品种为基于现有系统的保守实现选择，已在进度消息明确。
