---
id: CHG-2026-09-21-UPDATER-PROGRESS-VERIFICATION
result: passed
authority: evidence
---

# 行情更新进度：验证记录

生命周期见 [需求](requirements.md)，批准内容见 [设计](design.md)。验证生产代码基线为 `bfc4fbf`，包含 main 至 `3cc9c2c` 的工作台改动；后续收尾仅修改文档。

## 需求追踪

| 需求 | 设计 | 实现与验证 | 结果 |
|---|---|---|---|
| REQ-UP-001 | 4、5 | `market_scheduler.go` 的 TriggerTracked、API 受理回执与客户端契约测试；前端超时不重复 POST | passed |
| REQ-UP-002 | 3、4、6 | `refresh_progress.go` 记录股票/期货累计快照；状态、计数、失败及五小时模拟心跳测试 | passed |
| REQ-UP-003 | 1、2、7 | 调度器仅写观察事件，查询入口独立；连续两次采集测试证明历史不跳过证券 | passed |
| REQ-UP-004 | 3、4 | MySQL 重启中断、恢复失败后重试与固定启动边界测试；前端重新查询持久记录 | passed |
| REQ-UP-005 | 6 | RefreshMonitor 入口、股票按钮、防重、历史选择、关闭/隐藏行为测试 | passed |
| REQ-UP-006 | 5、6 | API 错误脱敏、游标校验与 MySQL 分页；前端失联保留记录、心跳过期与失败分页测试 | passed |
| REQ-UP-007 | 1、4 | 全量调度、采集、限频与发布回归通过；原有采集选择及窗口逻辑保持 | passed |
| REQ-UP-008 | 3.2 | 写入失败不阻断采集、重复保存幂等、陈旧快照拒绝、终态保护及最多 64 个待保存观察记录测试 | passed |

## 执行证据

执行 `bash scripts/verify.sh --full`，采用用户给定的本机代理与 Docker host 代理。日志位于本机 `/tmp/updater-final-full.log`（workbench 镜像初次失败，单独复验见下文），不将包含环境信息的完整日志纳入仓库。

- 前端 27 个测试文件、235 个测试通过；语句 93.59%、分支 87.77%、函数 90.54%、行 96.03%；TypeScript 与生产构建通过。
- Go 全量测试通过；总覆盖率 85.8%；market 94.3%、indicator 92.5%、strategy 94.8%、backtest 90.4%。
- 文档契约、全量 Race Detector、go vet、容器配置安全检查通过。
- 全市场扫描性能门禁通过：扫描约 177ms，批量读取 1 次，快照 p95 约 1.583μs（本机测试数据，非 NAS 性能承诺）。
- 获批远端 MySQL 8.4/x86_64 兼容性检查通过；在随机隔离测试库执行 MySQL 与 data 全套集成测试，通过。未以 SQL mock 代替数据库验收，未改写业务行情库。
- Docker updater/workbench 两个 linux/amd64 目标均实际构建并载入成功；检查非 root app 用户、默认启动参数、无真实配置；updater 不含前端，workbench 含 index.html。

首次完整数据库验收发现已有图表配置集成测试按原始 JSON 字符串比较，MySQL 返回的规范化空白使断言失败；独立提交 `bfc4fbf` 将两处比较改为 JSON 内容等价，目标测试及完整数据库套件复验通过，未改变生产语义。

镜像首轮构建中 npm 跳过两个下载失败的可选原生包（TypeScript Linux arm64、rolldown Linux arm64 musl），导致 workbench 构建失败。通过用户代理下载对应 tarball，逐一校验与 package-lock.json 的 SHA-512 一致，补入本地 npm 构建缓存；随后使用原 Dockerfile、相同 linux/amd64 目标及 `--no-cache` 重跑 workbench。没有修改依赖版本或生产代码；一次复验遇到 Alpine 软件源 TLS 错误，继续以相同原始构建命令重试；该轮前端和系统依赖构建通过，但 Go 下载长时间停滞。停止该次下载后，执行 `go mod verify`（全部通过），将本项目所需的本机模块归档补入构建缓存。最终用原 Dockerfile 重新构建 workbench（不再带 `--no-cache`，复用该轮成功步骤），编译、导出和载入成功。日志分别为 `/tmp/updater-workbench-image-retry2.log` 与 `/tmp/updater-workbench-image-final.log`。因此完整门禁由全量脚本已通过部分和镜像单独复验共同覆盖，不声称首次 `--full` 一次成功或最后一次 workbench 为全无缓存构建。

## 独立评审与修复

独立子 Agent 评审架构边界、并发、重启、页面轮询及遗留清理。发现并通过失败测试复现后修复：过期 RUNNING 锁死手动按钮；延迟首次落库的旧任务遮挡新任务；启动中断标记失败后未重试。

实现采用时间推进的失联提示、按 started_at/id 选择最新任务、固定启动边界的恢复重试。评审复验及主分支集成后的收尾复审均无阻塞问题，独立重跑 API/application 的 TestRefresh 系列通过。额外回归确保未知数据库异常不会进入响应或日志。

## 验证边界

五小时测试使用 Go testing/synctest 推进时间，覆盖持续心跳和最终完成；未在 NAS 实机执行五小时采集。远端验收使用隔离库，不代表已替用户升级 NAS 业务库。前端通过组件行为、可访问性断言与生产构建验收，未进行浏览器视觉验收。

长期规则已回填系统架构、application/port/MySQL/API/web 设计、HTTP 契约和运行手册；新增表只用于观察，不是断点或自动重试依据。

## 发布产物

新版 updater 导出到 `/Users/lmm/Desktop/trading-updater-20260921-progress/trading-updater-linux-amd64.tar`，标签 `trading-updater:20260921-bfc4fbf` 与 `latest`，34,196,992 字节。归档 manifest 再次校验 linux/amd64、app 用户及默认命令 `/app/trading -service updater -config /app/config.yaml`。SHA-256：`6c91c796203778c6dba1cefa29b6527fac19065af7b7d8b452fb3de2ed22636c`；独立 `shasum -c` 通过。同目录提供配置示例和升级说明；真实配置未打包。
