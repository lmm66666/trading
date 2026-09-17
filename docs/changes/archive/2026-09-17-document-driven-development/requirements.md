---
id: CHG-2026-09-17-document-driven-development
status: implemented
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-17"
approved_revision: "d4dc69f14fd023a8026d4fe67991b40762a3bee6:docs/requirements/active/REQ-2026-004-document-driven-development.md"
amendment_revision: "01f9bcb:docs/changes/active/2026-09-17-document-driven-development/"
approved_scope:
  - "DOC-001 through DOC-006 and sections 3–5 of the approved migration specification"
  - "DOC-007: 2026-09-17 explicit user approval in this task of risk-based local/MySQL/image verification"
---

# REQ-2026-004：统一文档驱动开发规范

| 属性 | 内容 |
|---|---|
| 创建日期 | 2026-09-17 |
| 适用范围 | 全仓文档布局、开发流程与文档契约测试 |
| 基线 | 8693e59；远端 fetch 因 SSH 公钥认证失败，未确认远端最新状态 |
| 已确认意图 | 用户要求修复 skill 第 2–6 项、安装本机，并明确选择按 skill 标准目录统一迁移，保留现有测试、安全与用户裁决门禁 |
| 批准证据 | 用户于 2026-09-17 在当前任务明确回复“批准，按该方案执行”；批准原始需求与目标设计；批准前完整内容 SHA-256：`1a6006f65f2aa2deb3cf8c78098107274410aefecc0503419244b67fc8999e28` |

## 1. 问题、目标与使用条件

现有项目在各源码模块内保存 DESIGN.md，在 docs/requirements 中保存单文件需求，并在 AGENTS.md 内重复通用开发流程。用户选择统一为 document-driven-development skill 的文档模型。

本次对象为单一 Go/React 仓库、14 份模块设计、3 份历史需求和现有架构、API、运行手册。文档在设计/实现/评审时读取，不新增服务、数据库或运行时调用，无需为文档访问频率建立缓存或平台。保持已有业务设计内容；此次迁移不是重新从代码提取或重新批准全部业务语义。

## 2. 稳定需求与非目标

- DOC-001：14 份模块设计集中到 docs/design，建立能力目录和带职责注释的源码树，每个实质源码文件有唯一 owns 归属。
- DOC-002：跨模块工程约束集中到 docs/standards，AGENTS.md 保留项目入口、项目门禁与导航；HTTP 契约迁至 docs/standards/http-api.md，运行手册仍为 docs/operations.md。
- DOC-003：新复杂变更使用 requirements.md、design.md、verification.md 三文件，审批版本、变更状态与检查结果分离；只归档已验收完成的变更。
- DOC-004：机械维护、轻量缺陷、复杂变更三条路径明确；用户裁决门禁、80% 总覆盖率和 90% 核心领域覆盖率、远端隔离 MySQL 和 amd64 镜像门禁保持不弱化。
- DOC-005：统一所有有效链接及文档契约测试，保留历史需求原始决策和验收，不伪造历史批准版本或补造测试证据。
- DOC-007：默认本地门禁，按风险显式选择 --mysql/--image/--full，保留所选门禁失败语义与未选择提示；具体批准修订见下文。
- DOC-006：可复用模板与通用流程由本机 skill 提供；仓库移除模板库。无业务代码、API 行为、数据库、策略或引擎语义变化。

非目标：全仓业务重新设计、修改已发现但未裁决的业务差异、升级依赖、建立文档网站、外部发布或推送。

## 3. 方案比较与选择

选择：集中迁移到 skill 标准目录，补充元数据与证据索引，保持业务内容。用户已经明确选择该方案。

放弃仅保留旧目录更新流程：不能满足统一目录的选择。放弃从代码重新提取所有业务设计：会扩大范围，并可能把已知缺陷升级为规范。

## 4. 可执行验收

| 需求 | 验证 |
|---|---|
| DOC-001 | 文档契约检查设计存在、owns 有效且所有实质源码恰好一个 owner；人工核查能力目录与源码树 |
| DOC-002 | 原规则逐条迁移对照；HTTP 正文无语义差异；所有 Markdown 链接及锚点通过 |
| DOC-003 | 新状态与审批结构契约检查，历史记录按显式 legacy 规则校验 |
| DOC-004 | 前后门禁对照；npm --prefix web run check、go test ./...、go vet ./...、bash scripts/verify.sh |
| DOC-005 | 历史需求仅导航/迁移注记变化；独立 Agent 按需求→设计→测试→代码顺序评审 |
| DOC-006 | Git diff 确认业务生产代码未变，旧模板及重复文档清理 |
| DOC-007 | TestVerificationSelection：默认、选项、组合、帮助、非法参数、失败传播；真实执行默认本地门禁 |


## 目标设计与验证

目标设计见 [design](design.md)，实际检查和追踪关系见 [verification](verification.md)。本文是变更整体生命周期的唯一来源。批准证据为当前任务中用户明确回复“批准，按该方案执行”；批准前完整内容 SHA-256 为 `1a6006f65f2aa2deb3cf8c78098107274410aefecc0503419244b67fc8999e28`。原始已批准完整需求与设计保存在上方 Git 修订中，三文件拆分不改变批准范围。

## 2026-09-17 已批准修订：按风险选择外部验收（DOC-007）

用户明确指出 MySQL 不可达且每次验证过重，并批准：“按变更风险选择，默认本地检查（推荐）”。此决定替代本需求初版要求每次都执行远端数据库与镜像的规则，不改变触发这些门禁时的验收强度。

- `bash scripts/verify.sh`：默认运行现有 1–8 项本地门禁；不访问远端 MySQL，不调用 Docker。原有覆盖率、Race、静态、性能、安全检查保持。
- `--mysql`：本地门禁后追加现有 deployment 预检与 integration 隔离库测试。涉及 SQL/模型/索引/迁移/事务/锁/队列持久化或数据库驱动变化时必须选择；仍禁止现有业务库与本地 MySQL 代替。
- `--image`：本地门禁后追加现有 linux/amd64 镜像构建。Dockerfile、构建依赖、打包或部署方式变化时必须选择。
- `--full`：上述两项全部执行；也允许 `--mysql --image` 组合。CI/发布可显式选择全量检查。
- `--help`/`-h`：显示用法，不运行门禁；未知参数以非零状态退出，不能静默忽略。
- 未选择的外部门禁在输出中明确显示“未选择”，脚本只宣称所选门禁通过；适用性由变更评审判断，不根据 git diff 自动猜测。
- 任何所选命令失败保留非零退出码，不输出成功。本次文档迁移和选项编排不改变 SQL、驱动或镜像内容，两项外部门禁均不适用。已有连接超时留在验证历史，不作为当前适用门禁阻塞。
- 脚本选择通过隔离命令替身测试默认/选项/组合/帮助/非法参数/失败传播；替身只验证编排行为，不能声称真实数据库或镜像验收通过。实际执行默认本地门禁。

批准证据来自当前任务的明确选项回复；精确修订内容由本次修订提交绑定，不回写成初始批准规格的一部分。
