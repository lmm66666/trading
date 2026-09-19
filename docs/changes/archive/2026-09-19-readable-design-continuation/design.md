---
approval_status: approved
approved_by: user
approved_at: "2026-09-19"
approved_revision: "conversation:documentation-redesign-handoff+user-继续指令"
approved_scope:
  - "第二至四批业务阅读层与入口收敛方案、skill 回写方式；不包含新业务语义"
---

# 目标设计：业务阅读层覆盖剩余链路并回写入口

## 1. 新增业务文档

沿用第一批定稿的结构（解决什么问题 → 完整走一遍 → 决定结果的规则 → 例子 → 数据与技术实现 → 验证与未决），frontmatter 一律 `kind: explanation`、`status: baseline-review`、`authority: code-derived`、`baseline_revision: de5a2120343878ba5078d2559e542463e4d67a88`、`owns: []`。

| 文件 | 内容边界 | 不包含 |
|---|---|---|
| `workflows/market-data.md` | 刷新触发、增量与全量、周线聚合、复权、版本与修订、失败与并发 | 表索引全集（归 MySQL 设计）、字段校验（归 HTTP 契约） |
| `workflows/backtesting.md` | 创建固定、执行顺序、信号与成交时刻、费用与滑点、持有期、公司行动现状、指标解释、失败取消复查 | 撮合与账户内部不变量（归回测模块设计） |
| `workflows/chart-query.md` | 查询链路、向前分页、指标计算位置与预算、失败边界 | 前端组件状态（归 web 设计） |
| `strategies/weekly-b1.md` | 对齐式判断顺序、日线辅助对齐、参数与例子 | 扫描/回测公共流程 |
| `strategies/bottom-surge-pullback.md` | 低位区启动、渐进与单日、surge_gap 延续、回撤窗口、J 区间过滤、例子 | 同上 |

规则归属不迁移：业务文档描述现状并链接技术契约；全局不变量仍在 `system-design.md`；HTTP 字段全集仍在 `standards/http-api.md`。

## 2. system-design.md 重写

- 第 2 节从"分层清单"改为"数据的一生"叙事，按采集→存储→计算→编排解释每个模块存在的原因；依赖方向与模块地图不变，仍链接 `design/README.md`。
- 全局业务不变量（3.1–3.5）逐条保留，补充"发布是增量观察"一条与既有行为一致；第 4 节流程改为索引+链接业务文档。
- frontmatter、`owns`、权威范围不变；`最后更新` 改为 2026-09-19。

## 3. 入口收敛与事实同步

- `design/README.md`：导航表补 5 个新文档；"推荐阅读路线"从行情采集开始；删除"当前只完成扫描和日线 B1"的过时说明；待判断问题表补公司行动、增量窗口、涨跌停数据三项。
- 技术文档入口：market、backtest、broker、web、api 在标题后加一句业务阅读指引（与应用层、策略模块第一批样式一致）。
- `strategy-scan.md` 采集链接由系统设计改为 `market-data.md`。
- `documentation_test.go` required 清单加入 5 个新文档；元数据规则不变。
- AGENTS.md 项目描述、operations.md 启动迁移清单、roadmap.md 愿景句删除已下线的财报能力表述（同步 CHG-2026-09-19-legacy-finance-macro-cleanup 的事实，无新语义）。

## 4. skill 更新（项目外）

本机 `$document-driven-development` skill 沉淀：业务流程/策略文档的分层阅读方式、frontmatter 边界（explanation 不拥有源码不覆盖契约）、"现状说明 vs 批准契约"的区分、以及"一条规则一个完整位置"的去重原则。先在项目内稳定样例再回写 skill；不在项目内复制模板。

## 5. 交接文档处置

`docs/documentation-redesign-handoff.md` 完成使命后删除：批次全部落地、验证记录归档，其内容已回写导航与变更记录，保留会与现状漂移。

## 6. 不做的事

- 不为已下线的财报/宏观能力补业务文档。
- 不把任何 explanation 文档升级为 approved/normative。
- 不修改生产代码；不改 MySQL/镜像门禁的适用性判断。
- 不静默裁决文档整理中发现的业务口径问题（公司行动、复权差异、涨跌停数据等只记录）。
