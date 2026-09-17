---
id: CHG-2026-09-17-document-driven-development-VERIFICATION
result: passed
authority: evidence
---

# 验证记录

整体状态见 [requirements](requirements.md)，批准目标及用户修订见 [design](design.md)。

## 追踪矩阵

| 需求 | 设计 | 实现/文档 | 检查 | 结果 |
|---|---|---|---|---|
| DOC-001 | 迁移矩阵、文件归属 | docs/design、系统设计 owns | TestDocumentationOwnership；源码树和能力目录人工评审 | passed |
| DOC-002 | 迁移矩阵 | 工程标准、HTTP 契约、AGENTS | 正文保留逐项对比、TestDocumentationLinks | passed |
| DOC-003 | 权威与审批 | 三文件记录、AGENTS | TestDocumentationContract、负向变异检查 | passed |
| DOC-004 | 按风险验收 | 工程标准 | bash scripts/verify.sh 默认本地门禁 | passed |
| DOC-005 | 历史迁移 | archive/legacy、documentation_test.go | 历史正文对比、独立评审 | passed |
| DOC-006 | 非目标、模板归属 | skill、Git diff | 业务生产代码零差异、旧模板移除 | passed |
| DOC-007 | 已批准风险分级修订 | scripts/verify.sh、verification_test.go | TestVerificationSelection 十组场景、真实默认执行 | passed |
| 外部 MySQL | DOC-007 适用性 | 远端预检与隔离库验收保留原命令 | 本次仅文档与验证编排，不改 SQL/模型/驱动/事务 | not-required |
| amd64 镜像 | DOC-007 适用性 | 镜像验收保留原命令 | 本次不改 Dockerfile、构建依赖、打包或部署内容 | not-required |

## 已执行检查与结果

2026-09-17，实际实现与门禁对应提交 `ec239db`；初始目标审批见 `d4dc69f`，用户追加的风险分级修订见 `01f9bcb`。最终归档仅改变状态、记录与路径链接，另跑文档契约验证。

- 迁移前 `go test . -run '^TestDocumentation' -count=1` 通过。先更新文档契约测试再执行：如预期因新文档缺失、旧目录残留和源码无 owns 失败；迁移后通过。
- 五项负向变异分别移除 owner、增加重叠 owner、把未验收变更标成完成、给设计复制生命周期、删除批准人：各自被门禁拒绝；恢复原文后文档测试通过。
- 14 份模块设计、HTTP 契约和 3 份历史 REQ（共 18 份）逐项与 `8693e59` 比较：移除新增元数据/历史注记并归一化链接后，正文完全一致。
- 新脚本选择测试先在旧脚本上观察失败，修改选项后 `go test . -run '^(TestVerificationSelection|TestDocumentation)' -count=1` 通过。10 组场景覆盖默认、mysql、image、full、组合、help、非法参数及三类失败传播。外部命令替身仅证明编排，不证明数据库或镜像正确性。
- `bash -n scripts/verify.sh`、`git diff --check` 通过。
- `bash scripts/verify.sh` 默认实际执行成功（退出码 0）：前端 7 个测试文件/21 个测试、生产构建、文档契约、Go 全量、覆盖率、Race、go vet、5000 证券性能、容器配置安全检查均通过；没有调用远端 MySQL 或 Docker。
- 前端语句覆盖率 93.06%；Go 总覆盖率 85.6%；market 94.3%、indicator 91.3%、strategy 94.8%、backtest 90.4%。
- 全市场性能检查耗时 172.34ms、批量读取 1 次。
- Skill：quick_validate.py、内部相对链接和安装副本字节一致性检查通过；独立 Agent 完成 7 个审批、轻量缺陷、基线、审计、阻塞与语义修订场景的桌面推演，无阻塞性矛盾。Skill 安装到本机 skills 列表。推演不替代项目测试。

## 审查

独立 Agent 按需求 → 设计 → 测试 → 代码审查迁移，未发现必须修复问题，并独立复核文档正文保留、实际运行文档契约测试。用户追加 DOC-007 后进行第二轮独立审查，实际运行十组脚本选择测试通过；默认无外部调用、失败退出、门禁与文档一致性均通过。

## 历史尝试与限制

初版全量脚本的本地 1–8 项通过，第 9 项远端 MySQL 预检连接超时，未进入隔离数据库测试；Docker 初始 daemon 不可用，镜像构建未执行。用户随后明确批准 DOC-007，使这两项对本次变更不适用：保留失败历史，不改写成通过，也不再作为适用门禁阻塞。未来触发对应风险仍须真实执行，不能以本次替身测试替代。

git fetch github 因 Permission denied (publickey) 失败；本次基于本地 main 8693e59，未确认远端最新状态，不推送远端。该限制与代码/文档验收结论分开记录。
