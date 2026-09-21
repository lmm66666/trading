---
id: CHG-UPDATER-EMBEDDED-CONFIG
status: implemented
approval_status: approved
authority: proposed
approved_by: user
approved_at: "2026-09-21T15:02:17.087924+00:00"
approved_revision: session-updater-embedded-config-v1
approved_scope: ["NAS updater 内置 config.updater.yaml，默认启动，更新 Dockerfile 并提交"]
---

# NAS 内置配置镜像

用户明确说明 NAS 不支持文件挂载，要求将准备好的 config.updater.yaml 内置镜像，并更新 Dockerfile 提交。该直接指令批准本节 v1 的部署方式例外，覆盖旧规则在此私有 updater 镜像上的“不得内置配置”限制；普通镜像仍不内置配置。

- REQ-EC-001：私有 updater 镜像包含用户配置，默认从 /app/config.yaml 启动，无需挂载或附加参数。
- REQ-EC-002：原普通 updater/workbench 构建继续可用；配置不提交、不进入普通构建上下文；更换配置重新构建必须生效。
- REQ-EC-003：导出 linux/amd64 tar，校验配置一致、app 用户可读，提交构建入口与使用说明。

沿用一个 NAS updater 和现有数据库，不调整应用、采集、表结构或网络暴露。验收不启动真实采集。
