---
id: CHG-2026-09-21-UNIFIED-MARKET-REFRESH-DESIGN
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T23:58:06+08:00"
approved_revision: "sha256:0451653fe5a950b98912084b49fda37176787b48df655c8a1eec39e409d10bfa"
approved_scope: [REQ-U01, REQ-U02, REQ-U03, REQ-U04]
---

# 统一行情更新设计

## 当前到目标
旧无身份 POST 只调用股票 TriggerTracked，期货只支持定时 RunOnce。目标由应用层统一调用两类调度器，复用各自原有 guard；期货增加与股票相同的受管理异步 TriggerTracked/Wait。定时与手动共用 guard；定时任务来源仍 SCHEDULED，手动为 MANUAL。

## 数据流与存储
浏览器 POST → workbench 固定认证代理 → updater 根 context → 应用统一触发 → 股票/期货调度 → 原共享限频来源 → 原版本化行情落库。两类独立进度仍写 t_market_refresh_runs，以 run_id 关联失败表，查询仍用现有索引；不增加表、列、索引或整体父任务。

## 回执与错误
无身份 POST 返回 202，data 为 {stock: receipt, futures: receipt}。每类 receipt.status 为 ACCEPTED/ALREADY_RUNNING/DISABLED/FAILED；仅 ACCEPTED 有 run_id 及 progress_available，FAILED 仅提供固定脱敏 error_code。股票不可 DISABLED，期货禁用时 DISABLED。逐类尝试，某类已运行或失败不阻断另一类；入口参数/根 context 无效在触发前失败。单证券接口不变。代理严格校验两类回执，不透传上游字段；旧回执视为响应异常。

## 前端
顶栏入口下方非模态面板，通过 portal 避免父布局影响，窗口变化重新定位。标题下一个统一“立即更新”，说明“全市场股票与已启用期货”；两个卡片仅展示状态与详情。仅全部可启用类别均运行/本次受理待观察时禁用；仍有闲置类别可再次触发。受理后记录短暂防重点，匹配终态立即解除，未知/过期超过60秒允许人工重试，由服务端 guard 最终防重。成功回执逐类展示，不把其他任务当成本次受理。查询不可达保留最后记录；超时/断线明确待核实，重新查询只 GET、提示可关闭。未启用期货明确显示未启用，未知能力说明无法读取状态。

## 生命周期、安全与容量
手動任务使用 updater root context，HTTP 返回不取消任务；服务关闭先停 HTTP/定时调度，再等待两类手动任务退出、关闭数据库。复用有限 worker、八品种顺序采集与共享限频。无需队列、跨实例锁或新缓存。NAS 能力查询沿用两秒超时；不以数据库空记录推断 NAS 可达。

## 空记录修复
最新任务与指定任务使用有界 Find/RowsAffected 区分缺失，向上仍返回 ErrRefreshRunNotFound；避免正常空值被 GORM 当 SQL 错误记录，真正 SQL 错误不吞。

## 迁移与替代
无数据库迁移；停止旧服务后同步升级前后端和 updater，回滚也同步。已批准不保留股票旧接口，不新增两次 POST 的前端拼接，避免半途响应混淆。

## 验证
调度器覆盖异步期货、共享防重、根 context 取消与 Wait、各类别组合和独立失败；API 覆盖联合回执/代理严格校验及单证券回归；UI 覆盖单按钮、部分运行、禁用期货、提交/查询错误、定位/关闭/焦点；MySQL 真实随机隔离库覆盖空值无日志、真实错误、原排序。执行快速检查、受影响覆盖率、Race、--mysql；不触发镜像验收。复杂变更独立子 Agent review 后才合并。

## 影响文档
系统设计、应用层、端口、API、HTTP 契约、前端、MySQL、运行手册回填。源码归属仍归原模块，未新增目录。
