---
id: CHG-2026-09-19-readable-design-continuation
status: implemented
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-19"
approved_revision: "conversation:documentation-redesign-handoff+user-继续指令"
approved_scope:
  - "第二批：行情采集与版本、回测业务流程文档，并据此重写系统全貌"
  - "第三批：周线 B1、底部倍量回撤、图表查询业务文档；按合并后的最终代码范围处理（财报等能力已由 CHG-2026-09-19-legacy-finance-macro-cleanup 下线，不再为其编写业务文档）"
  - "第四批：导航与技术文档入口收敛、文档检查清单更新、AGENTS/operations/roadmap 与已合并清理任务的事实同步、更新本机 document-driven-development skill"
---

# 文档改造续批：补齐业务阅读层并回写全局入口

用户在 2026-09-19 会话中指示"文档改进计划，执行了一半，你继续；最后别忘了更新 codex 对应的 skill"。该指令延续已批准的第一批规划（CHG-2026-09-19-readable-design-pilot）与交接文档 `docs/documentation-redesign-handoff.md` 中列明的第二至四批范围；本变更不引入新业务规则，不改生产代码。

| 需求 | 验收 |
|---|---|
| READ-006 | 新增 `workflows/market-data.md`、`workflows/backtesting.md`、`workflows/chart-query.md` 与 `strategies/weekly-b1.md`、`strategies/bottom-surge-pullback.md`，格式与第一批一致（explanation / baseline-review / code-derived / 完整代码基线） |
| READ-007 | `system-design.md` 按完整链路重写模块分工说明，保留全局业务不变量的唯一权威位置，frontmatter 与源码归属不变 |
| READ-008 | 导航"我想知道什么"表与待判断问题表更新；market、backtest、broker、web、api 技术文档增加业务阅读入口；strategy-scan 的采集链接改指业务文档 |
| READ-009 | `documentation_test.go` required 清单纳入新文档；AGENTS、operations、roadmap 中与已合并财报清理不一致的表述同步为事实 |
| READ-010 | 更新本机 `$document-driven-development` skill，沉淀业务阅读层写作方式；项目内不复制模板 |
| READ-011 | 本地门禁通过并如实记录；文档-only 不新增业务测试；不伪造批准与验证证据 |

使用条件：单仓库文档与 skill 维护；基线为本地 main `de5a212`（已合并财报清理）。不覆盖另一任务的产物，不改变任何已批准业务语义；发现的新未决业务问题只记录，不裁决。

目标设计见 [design](design.md)，实际证据见 [verification](verification.md)。
