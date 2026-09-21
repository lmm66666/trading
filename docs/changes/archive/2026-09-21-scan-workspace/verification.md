---
id: CHG-2026-09-21-scan-workspace-VERIFICATION
result: passed
authority: evidence
---

# 扫描工作台验证记录

生命周期见 [需求](requirements.md)，批准目标见 [设计](design.md)。批准版本 `5b52fe7`；最终生产源码验证版本 `0e69c6e`。2026-09-21 完成验收与独立复核。

## 需求追踪

| 需求 | 设计 | 实现 | 验证证据 | 结果 |
|---|---|---|---|---|
| SCAN-001 | 2 | ScanForm、styles.css | 表单回归；Chrome 桌面与 375/400px 视口检查 | passed |
| SCAN-002 | 1、4 | ScanPanel、useRunPolling | 有 ID 首屏 loading、失效引用回归；浏览器小图标 | passed |
| SCAN-003 | 4 | ApiError、useRunPolling、目录 retry | 404 一次停止、网络三次停止及重连、旧响应隔离；终态结果连接错误可见 | passed |
| SCAN-004 | 3、4 | ScanPanel、ScanResults、scanPreferences | 提交冻结条件、异步编辑不覆盖、新快照失败保留旧结果、零行成功替换 | passed |
| SCAN-005 | 2、3 | ScanResults | 固定 key 分页、已加载数量、旧页失败/成功隔离、部分失败；真实部分成功任务 | passed |
| SCAN-006 | 3、4 | App 保活及 active 控制 | ScanNavigation 集成验证草稿、分页、滚动容器；浏览器证券图表往返 | passed |
| SCAN-007 | 2 | ScanForm、RangePicker | 直接输入与快捷日期同步、默认参数冻结、中文标签、窄屏日历回归 | passed |
| SCAN-008 | 7 | 前端测试与标准门禁 | 完整 verify.sh、覆盖率与独立代码评审 | passed |

测试定位：`web/src/features/scan/ScanPanel.test.tsx`、`scanPreferences.test.ts`、`web/src/features/strategy/useRunPolling.test.tsx`、`RangePicker.test.tsx`、`web/src/ScanNavigation.test.tsx`、`web/src/api/client.test.ts`；原回测用例一并运行。

## 实际执行

- 基线 `a71c871` 前端 181 项测试通过；原工作区干净，git fetch origin 成功，未改动用户未提交内容。
- 测试先行：任务首屏/结构化错误、条件恢复/结果保留、隐藏弹层、评审三项问题及日历高度回归均观察到失败，再实现并通过。
- 最终 `bash scripts/verify.sh` 退出 0：前端 **207 项测试通过**、生产构建通过、文档门禁、Go 全量、覆盖率、race、go vet、性能及容器配置安全检查全部通过。
- 前端覆盖率：statements **91.94%**、branches **86.79%**、functions **88.61%**、lines **94.13%**。
- Go 总覆盖率 **86.6%**；market **94.3%**、indicator **92.5%**、strategy **94.8%**、backtest **90.4%**。
- 性能门禁：full_scan=171.836041ms，batch_reads=1；仅为标准测试夹具，不作为实际数据库耗时承诺。
- MySQL 门禁：not-required，无 SQL、表/索引、事务、迁移、驱动或后端队列变化；浏览器访问现有服务的烟测不冒充隔离数据库验收。
- 镜像门禁：not-required，无 Dockerfile、构建依赖或打包部署方式变化。

## 浏览器验收

用户已批准使用浏览器。Chrome 通过 worktree Vite 预览连接本机已有 8080 服务：

1. 桌面空态为有明确尺寸的小图标，顶部常用条件、折叠参数和高级设置可操作。
2. 选择近一年范围并提交真实扫描，状态从提交中进入运行中，再返回 PARTIAL_SUCCEEDED：70 只入选证券、1 只处理失败；页面正确区分并展示。
3. Chrome 响应式工具 400px 与 375px 检查：条件换行、按钮可见、页面不发生横向溢出，结果表在自身容器内横向滚动。
4. 375px 日期弹层发现预设栏被压成竖排，最小修复 flex 分配并增加实际剩余高度上限；重新检查后预设、月历与确认操作可读可用。
5. 入选证券进入图表后返回，原条件与 70 条结果保持。
6. 开发期间一次全量 npm ci 使 Vite 热更新模块失效，重新载入后恢复；生产构建独立通过，不把该开发环境事件当作产品缺陷。

原“巨大放大镜”的运行时现象未复现，不能声称已经确定旧构建/缓存为根因。此次明确修复了恢复中误展示空态的路径，并用 SVG width/height 与 CSS 双重限制尺寸；NOT_FOUND 清理路径先由回归证明，未删除业务任务以人为制造 404。合并后在原 8080 页面进一步实际命中已保存的失效任务，正常显示“上次扫描记录已不可用，可重新发起扫描”，自动恢复可提交空态，证实该运行时恢复路径。

## 独立评审

独立 Agent `scan_review` 审查基线至 `fa6ab96`，发现 3 项 P2：目录不可重试、终态保留时连接错误隐藏、有效默认参数缺失。`90830f2` 修复并补齐先失败后通过的回归，独立复核确认三项闭环，另运行 54 项相关测试通过。最终 `90830f2..0e69c6e` 日历差异再次复核，未发现新增问题。

当前前端设计已回填，无未决阻断项。原有策略预热/数据新鲜度待决语义不在本次范围。未推送远端；按用户仓库规则本地合并 main。

## 本地集成验收

main 快进合并后重新构建 `web/dist`、文档检查通过；Chrome 已打开原 8080 服务并确认新版顶部条件布局及失效任务恢复生效，无需重启 Go 服务。仅更新本地工作台，未推送远端。
