---
id: CHG-2026-09-21-RETURN-ZSCORE-VERIFICATION
result: passed
authority: evidence
---

# RETZ 验证记录

按[需求](requirements.md)和[设计](design.md)继续完成既有 worktree，用户在本任务明确要求继续开发。保留原批准元数据，不补造历史批准时间。

## 需求追踪

| 需求 | 实现 | 证据 |
|---|---|---|
| REQ-RETZ-001 | return_zscore.go、graph.go | TestLogReturnSeriesComputesLogReturnsAndHandlesGaps、TestReturnZScoreSeriesUsesPopulationStandardDeviation |
| REQ-RETZ-002 | Ref、graph、chartIndicatorRefs | TestReturnZScoreComponents、TestChartQueryExpandsRETZComponents、TestQueryChartRETZTransport |
| REQ-RETZ-003 | 应用参数/成本校验、前端指标管理 | TestChartQueryRejectsInvalidRequests、TestLegacyIndicatorsRejectRETZParameters、IndicatorManager.test.tsx |
| REQ-RETZ-004 | FinancialChart | FinancialChart.test.tsx：同pane、阈值边界颜色、线色、五条参考线与σ图例 |
| REQ-RETZ-005 | 前缀/分页/看板身份及读写 | TestEveryBuiltInFeatureIsPrefixInvariant、TestRETZPaginationMatchesFullHistory、TestChartBoardRETZIdentityAndRoundTrip、useBoards.test.tsx、chartData.test.ts |

## 口径说明

原设计 Test strategy 中“126+5+252”是与其正文及 REQ-RETZ-003 不一致的算式笔误；按两处明确的 `band + regime` 实施，不把 smooth 计入成本。原超预算测试的五个组合实际未超过2000，已修正为六个；前端另验证未超限组合仍可添加。原需求中的“日”在 WEEK 下按其明确非目标解释为逐周涨幅。

## 门禁

- Go 指标/应用/API 测试已通过。
- 前端169测试、构建通过；语句91.87%、分支84.37%、函数88.83%、行94.04%。
- `bash scripts/verify.sh` 全部所选门禁通过：Go全量、文档、Race、go vet、性能和容器配置安全检查。Go总覆盖率87.0%，market94.3%、indicator93.4%、strategy94.8%、backtest90.4%；性能门禁全市场扫描171ms、一次批量读取。独立 review 完成；发现并修复参考线不参与自动缩放导致±2σ不可见的问题，补充范围、极值、null及null priceRange测试，并重跑前端 check 通过。
- MySQL 门禁not-required：仅扩展已有 JSON 配置的应用校验与序列化，不改变 SQL、持久化模型、索引、DDL、事务/锁/队列或数据库驱动；存储透传路径不变，应用层往返有回归测试。独立评审确认。
- 镜像门禁not-required：不改变构建依赖、打包、Dockerfile 或部署方式。独立评审确认。
- 未进行浏览器视觉验收；通过组件测试检查传给图表库的绘制数据/选项及实际DOM图例。

## 接续开发的回归证据

新增测试先复现看板错误去重、旧指标接受新字段、RETZ配色缺失、mock键格式不一致、参考线自动缩放缺失，随后最小修复并通过。已保留未完成分支的算法/测试成果，未重写既有功能。未执行远端推送。
