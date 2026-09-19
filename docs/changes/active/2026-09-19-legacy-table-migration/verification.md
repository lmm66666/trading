---
id: CHG-2026-09-19-legacy-table-migration-VERIFICATION
result: pending
authority: evidence
---

# 验证记录

整体状态见 [requirements](requirements.md)，批准目标见 [design](design.md)。

## 追踪矩阵

| 需求 | 设计 | 实现/文档 | 检查 | 结果 |
|---|---|---|---|---|
| LTM-001 | §1.1 | 迁移器旧表直迁、因子/公司行动空 | 迁移器单测；`go test ./...` | pending |
| LTM-002 | §1.3 | eastmoney 文件、MarketSource 删除 | 全仓 grep；编译 | pending |
| LTM-003 | §2 | 共享函数搬移重命名 | `go vet ./...`；broker 测试 | pending |
| LTM-004 | §1/§3 | 设计文档、地图、roadmap 同步 | TestDocumentation*；人工核对 | pending |
| LTM-005 | §1.2 | dry-run 全量验收 | dry-run 输出：无 Failures、Quality COMPLETE | pending |
| 门禁 | — | verify.sh | 完整输出记录 | pending |
