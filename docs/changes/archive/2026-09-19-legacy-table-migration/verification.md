---
id: CHG-2026-09-19-legacy-table-migration-VERIFICATION
result: passed
authority: evidence
---

# 验证记录

整体状态见 [requirements](requirements.md)，批准目标见 [design](design.md)。

## 追踪矩阵

| 需求 | 设计 | 实现/文档 | 检查 | 结果 |
|---|---|---|---|---|
| LTM-001 | §1.1 | 迁移器旧表直迁、因子/公司行动空 | `TestLegacyBackfillConvertsRowsDirectly`、`TestLegacyBackfillRejectsInvalidLegacyRows`、`TestLegacyPriceScalesDecimalToFixedPoint`；`go test ./...` 全绿（2026-09-20） | passed |
| LTM-002 | §1.3 | eastmoney 文件、MarketSource 删除 | 全仓 grep 无 `EastmoneyMarketSource`/`port.MarketSource`（2026-09-20）；编译与全量测试通过 | passed |
| LTM-003 | §2 | 共享函数搬移重命名 | `go vet ./...` 通过；`broker_test.go` 覆盖传输分类与定点解析；独立评审逐行比对确认纯重命名无行为变化 | passed |
| LTM-004 | §1/§3 | 设计文档、地图、roadmap 同步 | `go test -run TestDocumentation .` 通过（2026-09-20）；人工核对 6 个文档与代码行为一致 | passed |
| LTM-005 | §1.2 | dry-run 全量验收 | dry-run 完整输出：见下节 | passed |
| 门禁 | — | verify.sh | 见下节 | passed |

## 验证执行记录（2026-09-20）

- `npm --prefix web run check` 通过。
- `go test ./...` 全部通过（含迁移器单测、broker 测试、集成测试编译标签外的全部包）。
- `go vet ./...` 通过。
- `bash scripts/verify.sh` 所选门禁全部通过（容器配置安全检查、全市场性能门禁 full_scan=188ms）；`--mysql` 未选择：本变更无内核持久化语义修改（迁移器写路径语义由 sqlmock 单测与既有 `-tags=integration` 集成测试覆盖，未触发工程标准的 MySQL 风险条件）；`--image` 未选择：无构建或部署变化。
- `go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true -batch-size=1000`：第一次暴露旧表存量数据问题（见下节），修复后第二次通过（见下节）。

## dry-run 报告摘要

第一次全量 dry-run（2026-09-20，修复前）：4979 只证券，CompletedInstruments=4949，Failures=30（19 只科创板 688xxx INVALID_DATA + 11 只深市 002xxx INCOMPLETE_DATA），RejectedCodes=[302132]，Quality=INCOMPLETE，BacktestEnabled=false。

根因查库确认（均为旧表存量数据问题，直迁校验如实暴露）：
- 19 只科创板：周线 37 行非正价格（low=0 或上市首周全零聚合行）+ 日线 3 行全零行（2024-11-06，688089/688143/688173），触发 `ErrInvalidOHLC`（价格必须全为正）。
- 11 只 002389-002398、002407（航天彩虹、信邦制药等）：`t_stock_info` 有记录但日/周线零数据（旧采集器漏采），触发 `ErrMigrationIncomplete`。
- 302132（中航成飞，北交所）：K 线数据完整（1090 根日线），但迁移器 BSE 白名单（4/8/920）不含 2025 年启用的 302 段；broker 新浪映射与内核身份无前缀限制。

用户裁决（2026-09-20）：删除 40 行坏 K 线行后重跑（激活后首次全市场刷新从新浪全量拉取自动补回正确数据）；从 `t_stock_info` 删除 11 只零数据记录（接受不迁移，将来需要时另行补采）；迁移器白名单加入 302 前缀（commit 4edf85f，测试与设计文档同步更新）。

数据修复执行（2026-09-20）：删除 `t_stock_kline_weekly` 37 行、`t_stock_kline_daily` 3 行、`t_stock_info` 11 行；删除前备份至本地 `/tmp/legacy-fix-backup/`；删除后复查坏行计数归零。

第二次全量 dry-run（修复后，2026-09-20）：4969 只证券全部完成（CompletedInstruments=4969），DailyBarCount=5188968，WeeklyBarCount=1043011，RejectedCodes=[]，Failures=null，Quality=COMPLETE，退出码 0。数字对账：4969 = 4979 − 11（info 删除）+ 1（302132 转为正常完成）；日线 5188968 = 5187881 + 1090（302132）− 3（坏行删除）；周线 1043011 = 1042966 + 82 − 37。LTM-005 的 dry-run 验收达成；apply 与激活 SQL 按需求约定在用户停服窗口另行执行。

## 独立子 Agent 评审（2026-09-20）

结论：无 blocker、无 major。核心正确性确认：`legacyPrice` 边界处理正确（NaN/Inf 拒绝、±2^63 精确边界、×10000 溢出为 Inf 仍被拦截）；UTC 全日会话与生产 `ParseSinaDaily` 约定一致；`NewDataset` 兜底升序/重复/OHLC/负量；共享函数搬移为纯重命名。

minor 发现与处置：
- `legacyDate` 每行重复 `MapLegacyInstrument`（可优化为装载层一次校验）：性能影响可接受（dry-run 全量完成时间在预期内），按最小修改原则不在本变更处理。
- `docs/analysis/` 删除（7f6aed8）：系用户在 d9063b2 中自行提交的内容经 rebase 保留，非实施方擅自扩大范围。
- time.Parse 失败归入 `SOURCE_UNAVAILABLE`：设计批准范围内的分类选择（design.md §1.1“失败映射既有失败分类”），保持不变。
