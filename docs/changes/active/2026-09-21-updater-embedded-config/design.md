---
id: CHG-UPDATER-EMBEDDED-CONFIG-DESIGN
approval_status: approved
authority: proposed
approved_by: user
approved_at: "2026-09-21T15:02:17.087924+00:00"
approved_revision: session-updater-embedded-config-v1
approved_scope: ["NAS updater 内置 config.updater.yaml，默认启动，更新 Dockerfile 并提交"]
---

# 内置配置构建

增加显式 updater-configured 目标，继承 updater。构建以 BuildKit secret 接收配置，将其复制到 /app/config.yaml，所有者 app:app、权限 0400，最终仍以 app 运行。secret 是构建输入机制，最终镜像有意保留配置，不宣称凭据对镜像持有者保密。

Makefile 显式选择此目标，传入 config.updater.yaml，并用 --no-cache-filter updater-configured 避免 secret 内容变更不失效缓存。缺少构建输入必须失败。普通目标不依赖 secret；.dockerignore 保持排除真实配置；Git 忽略新增本地配置。Dockerfile 最后仍为 workbench，默认目标不变。

只更改打包入口及对应规则文档；用示例配置做镜像门禁，真实配置构建只校验内容一致和权限，不连接数据库。用户允许把配置内置私有 NAS 镜像的决定优先于原通用只读挂载规则。
