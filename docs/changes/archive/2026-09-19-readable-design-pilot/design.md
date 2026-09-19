---
approval_status: approved
approved_by: user
approved_at: "2026-09-19"
approved_revision: "conversation:01a0ba2d-e1bc-73a1-b0a5-6bdf424746c6:design-document-plan-before-user-嗯可以"
approved_scope:
  - "已同意的第一批文档分层、业务导航与去重方案；不包含新业务语义"
---

# 设计：先理解结果，再深入规则和实现

入口是 `docs/design/README.md` 的问题导航。先读 `workflows/strategy-scan.md` 理解结果含义、窗口、数据流与存储关系，再读 `strategies/daily-b1.md` 核对 B1 阈值、状态与例子。`internal/strategy.md` 保留公共策略技术契约。索引和两个模块入口链接新的说明，其他技术契约继续提供字段、索引和生命周期细节。

保留原技术契约与批准记录；新文档完整描述现状但不新增规范效力。新阅读目录使用 `kind: explanation`、`status: baseline-review`、`authority: code-derived`，绑定完整代码提交，不拥有源码。机器检查只对这两个目录接受上述组合，且拒绝缺少基线、声明源码归属或伪装为批准契约的说明。其余设计仍要求原有 approved/normative 状态，源码仍只有一个技术设计所有者。

这是对获准阅读结构的实现，批准来源为 requirements 中的对话规划；不声称此文件的新措辞已获逐字批准。新增现状说明和既有契约冲突时，明确列出差异并保留原契约。预热参与策略回放、新鲜度、重复信号等业务选择不在本次决定。

与“继续把所有内容塞进模块文档”相比，分开阅读主题降低首读负担；与另起整套技术文档相比，保留既有契约避免双份规则。迁移只涉及 Markdown 和已有文档检查，不引入新工具、运行时依赖或数据库变更。

验证先复现原门禁拒绝代码提取说明，再用正反例验证说明范围与技术契约边界；运行默认本地门禁，独立审阅新说明与源码、旧契约是否一致。回滚可还原本批文档与文档测试，不涉及运行数据。
