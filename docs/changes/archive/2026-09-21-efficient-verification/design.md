---
id: CHG-EFFICIENT-VERIFICATION-DESIGN
approval_status: approved
authority: proposed
approved_by: user
approved_at: "2026-09-21T15:21:14.999595+00:00"
approved_revision: session-efficient-verification-v1
approved_scope: ["按风险选择验收、减少重复与无缓存下载，保留必要质量检查"]
---

# 验收入口

无参数运行 Go 测试/vet、前端测试/构建。独立 --mysql；--image 可选 updater/workbench/updater-configured，未指定目标检查全部。--full 执行前端覆盖率、单次 Go 覆盖率并由同一 profile 提取核心包指标、Race、vet、MySQL、全部镜像。未知参数/目标在执行前拒绝。所选命令失败直接非零退出，不伪报完成。

前端依赖缺失时安装，依赖文件变化须显式 npm ci；不增加自研缓存状态管理。Docker 使用内容缓存，只对含 secret 的配置阶段强制失效。要求与触发矩阵保存在工程标准，AGENTS/操作手册仅同步入口。通过可执行命令替身验证调度、失败传播与覆盖率不重复；实际快速检查与必要镜像验收作为落地证据。
