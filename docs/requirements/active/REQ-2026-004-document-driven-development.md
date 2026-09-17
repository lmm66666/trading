# REQ-2026-004：统一文档驱动开发规范

| 属性 | 内容 |
|---|---|
| 状态 | 已批准 |
| 创建日期 | 2026-09-17 |
| 适用范围 | 全仓文档布局、开发流程与文档契约测试 |
| 基线 | 8693e59；远端 fetch 因 SSH 公钥认证失败，未确认远端最新状态 |
| 已确认意图 | 用户要求修复 skill 第 2–6 项、安装本机，并明确选择按 skill 标准目录统一迁移，保留现有测试、安全与用户裁决门禁 |
| 批准证据 | 用户于 2026-09-17 在当前任务明确回复“批准，按该方案执行”；批准本文需求与目标设计；批准前完整内容 SHA-256：`1a6006f65f2aa2deb3cf8c78098107274410aefecc0503419244b67fc8999e28` |

## 1. 问题、目标与使用条件

现有项目在各源码模块内保存 DESIGN.md，在 docs/requirements 中保存单文件需求，并在 AGENTS.md 内重复通用开发流程。用户选择统一为 document-driven-development skill 的文档模型。

本次对象为单一 Go/React 仓库、14 份模块设计、3 份历史需求和现有架构、API、运行手册。文档在设计/实现/评审时读取，不新增服务、数据库或运行时调用，无需为文档访问频率建立缓存或平台。保持已有业务设计内容；此次迁移不是重新从代码提取或重新批准全部业务语义。

## 2. 稳定需求与非目标

- DOC-001：14 份模块设计集中到 docs/design，建立能力目录和带职责注释的源码树，每个实质源码文件有唯一 owns 归属。
- DOC-002：跨模块工程约束集中到 docs/standards，AGENTS.md 保留项目入口、项目门禁与导航；HTTP 契约迁至 docs/standards/http-api.md，运行手册仍为 docs/operations.md。
- DOC-003：新复杂变更使用 requirements.md、design.md、verification.md 三文件，审批版本、变更状态与检查结果分离；只归档已验收完成的变更。
- DOC-004：机械维护、轻量缺陷、复杂变更三条路径明确；用户裁决门禁、80% 总覆盖率和 90% 核心领域覆盖率、远端隔离 MySQL 和 amd64 镜像门禁保持不弱化。
- DOC-005：统一所有有效链接及文档契约测试，保留历史需求原始决策和验收，不伪造历史批准版本或补造测试证据。
- DOC-006：可复用模板与通用流程由本机 skill 提供；仓库移除模板库。无业务代码、API 行为、数据库、策略或引擎语义变化。

非目标：全仓业务重新设计、修改已发现但未裁决的业务差异、升级依赖、建立文档网站、外部发布或推送。

## 3. 方案比较与选择

选择：集中迁移到 skill 标准目录，补充元数据与证据索引，保持业务内容。用户已经明确选择该方案。

放弃仅保留旧目录更新流程：不能满足统一目录的选择。放弃从代码重新提取所有业务设计：会扩大范围，并可能把已知缺陷升级为规范。

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

完整交付运行 bash scripts/verify.sh，沿用远端隔离数据库与镜像要求；依赖或环境失败准确记为 blocked/failed，不归档为 implemented、不宣称完成或覆盖率达标。远端同步未恢复时报告无法确认远端状态。

## 5. 可执行验收

| 需求 | 验证 |
|---|---|
| DOC-001 | 文档契约检查设计存在、owns 有效且所有实质源码恰好一个 owner；人工核查能力目录与源码树 |
| DOC-002 | 原规则逐条迁移对照；HTTP 正文无语义差异；所有 Markdown 链接及锚点通过 |
| DOC-003 | 新状态与审批结构契约检查，历史记录按显式 legacy 规则校验 |
| DOC-004 | 前后门禁对照；npm --prefix web run check、go test ./...、go vet ./...、bash scripts/verify.sh |
| DOC-005 | 历史需求仅导航/迁移注记变化；独立 Agent 按需求→设计→测试→代码顺序评审 |
| DOC-006 | Git diff 确认生产代码未变，旧模板及重复文档清理 |

## 6. 最终验收

待批准目标设计并执行后填写。当前未修改业务文档或生产代码。
