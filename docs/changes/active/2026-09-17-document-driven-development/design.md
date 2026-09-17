---
id: CHG-2026-09-17-document-driven-development-DESIGN
authority: normative
approval_status: approved
approved_by: user
approved_at: "2026-09-17"
approved_revision: "d4dc69f14fd023a8026d4fe67991b40762a3bee6:docs/requirements/active/REQ-2026-004-document-driven-development.md"
approved_scope:
  - "DOC-001 through DOC-006 and sections 3–5 of the approved migration specification"
  - "DOC-007: 2026-09-17 explicit user approval in this task of risk-based local/MySQL/image verification"
---

# 文档治理目标设计

需求与唯一生命周期见 [requirements](requirements.md)。本文件从用户批准的完整规格提取，不改变其语义。

## 4. 目标设计与迁移矩阵

| 原位置 | 目标位置 | 处理 |
|---|---|---|
| 各模块 DESIGN.md | docs/design/<源码目录>.md | 保留业务正文，修复相对链接，增加 owns/related 和已有基线来源 |
| docs/architecture/domain-map.md | docs/design/README.md | 保留模块边界和兼容依赖，增加能力目录、带注释源码树和唯一设计归属说明 |
| docs/architecture/system-design.md | 原位 | 保留系统流程，更新导航；组合根与 config 的 owns 在这里定义 |
| api/api.md | docs/standards/http-api.md | 保留公开 HTTP 契约，仅变更文档位置 |
| AGENTS.md 工程规范 | docs/standards/engineering.md | 迁移代码、错误、日志、安全、数据库和质量规则；AGENTS 链接此处 |
| docs/requirements/archived/REQ-*.md | docs/changes/archive/legacy/REQ-*.md | 保留历史格式、状态与原始证据，注明旧治理已被本变更取代；仅修复可导航链接 |
| 本需求及后续新复杂变更 | docs/changes/active/<date>-<slug>/ | requirements 作为整体状态唯一来源；design 存审批；verification 存检查结果与追踪矩阵 |
| docs/templates/ | 本机 skill 的 assets/templates | 删除仓库通用模板，入口指向 skill |
| documentation_test.go | 原位 | 测试先行，适配新路径和历史例外，检查唯一源码归属、元数据及有效链接 |

docs/adr 按需建立：本次治理迁移的取舍已在本变更中，不另造重复 ADR。未来真正有长期跨模块决策时可建立不可变 ADR。

### 4.1 权威与审批

只有明确批准、绑定可追踪内容版本的目标设计，才在其列明范围内覆盖当前设计。草稿无覆盖权。业务代码与设计或权威文档彼此冲突，沿用现有规则：暂停受影响路径，给出差异、两种处理及影响，由用户裁决；已有同范围裁决直接复用。

新变更 requirements.md 的 status 是整体生命周期唯一来源：draft → reviewed → approved → implementing → verifying → implemented；rejected/superseded 作为终止状态。design.md 用 approval_status；verification.md 用 result（pending/passed/failed/blocked），不复制整体状态。批准记录包含需求、目标设计及受影响规范的内容版本；语义变更需重新审批，纯链接迁移不重新审批业务行为。历史文档缺失的批准字段保留未知，不用本次治理批准补造。

### 4.2 文件归属

owns 使用仓库相对精确文件或以 / 结尾的目录；目录只覆盖直接文件。strategy/builtin、mysql/dbtest、web 子目录显式列出。model 归 data；旧指标辅助归 business，限频器文件归 broker；配置和组合根归系统设计；验证脚本归工程标准；历史 shell 脚本归相应调用模块。不得重叠声明。

### 4.3 验收与失败处理

先修改文档契约测试，使新布局缺失时明确失败，再迁移并验证通过。代码—设计冲突不因本次迁移而自动裁决；可独立的文档路径工作继续推进。链接损坏或归属冲突必须修复后合并。

按变更风险选择适用门禁：bash scripts/verify.sh 默认本地检查，--mysql 验证远端隔离数据库，--image 构建镜像，--full 执行全部。被本次变更触发的必要门禁失败或缺环境才记为 failed/blocked，阻止归档与合并；不适用项记为 not-required，并写明判断依据，不能伪报通过。远端同步未恢复时报告无法确认远端状态。


## 当前文档清单与验证范围

14 份包设计迁移至 `docs/design/<source-path>.md`；工程规则在 `docs/standards/engineering.md`，HTTP 契约在 `docs/standards/http-api.md`。AGENTS、系统设计、运行手册、Roadmap 与源码地图同步更新导航。历史归档保留正文和原验收事实，仅添加迁移注记并修复链接。

文档契约复用已有 Markdown AST 链接/锚点检查和项目 YAML 依赖，新增元数据与唯一归属检查；不引入依赖或运行时工具。设计文档继承基线 `8693e59` 的当前有效状态，以 `approval_provenance: inherited-current-design` 说明来源；未知的历史批准字段保持 null，本次批准不替代业务批准。

源码归属覆盖 Go/TS/TSX/JS/JSX/CSS/SQL/shell（含测试），排除依赖和生成产物。目录 owns 只覆盖直接文件，tests 与配置可跟随模块，跨模块共享依赖使用 related，不允许重叠所有者。检查证明链接和归属可解析，不能证明业务语义正确。

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
