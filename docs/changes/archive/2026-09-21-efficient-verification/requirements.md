---
id: CHG-EFFICIENT-VERIFICATION
status: implemented
approval_status: approved
authority: proposed
approved_by: user
approved_at: "2026-09-21T15:21:14.999595+00:00"
approved_revision: session-efficient-verification-v1
approved_scope: ["按风险选择验收、减少重复与无缓存下载，保留必要质量检查"]
---

# 高效验收

用户在确认慢的原因后明确要求“修改一下验收要求，要兼顾效率，没必要这么复杂”。此直接指令批准按风险精简验收，不降低业务正确性要求。

- REQ-EV-001：默认快速检查，不重复运行全量覆盖率与 Race。
- REQ-EV-002：数据库和指定镜像可以单独验收，不附带无关测试；显式 full 保留全量能力。
- REQ-EV-003：缓存依赖，配置层强制更新；失败传播准确，已通过证据可复用。

不改业务逻辑、数据库、采集或 UI。使用已有单用户 NAS/电脑部署背景，无额外使用量假设。
