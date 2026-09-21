---
id: CHG-2026-09-21-NAS-SERVICE-SPLIT-VERIFICATION
result: passed
authority: evidence
---

# NAS 服务拆分验证记录

变更状态由 [需求](requirements.md) 管理，目标见 [设计](design.md)。本记录的 passed 只表示用户调整后的代码交付范围通过，不表示 NAS 已部署或外部门禁通过。

## 验收范围与用户决定

用户批准设计后，在远端数据库预检超时时明确表示“现在连不上 nas。代码编译成功就行”。因此本次真实 MySQL、Docker 镜像和 NAS 部署验收豁免；后续部署仍需实际验证。Go 本机编译、linux/amd64 交叉编译、前端生产构建作为本次交付门槛。独立代码评审继续完成。

## 需求追踪

| 需求 | 实现 | 证据与结果 |
|---|---|---|
| REQ-SPLIT-001/002 | main.go、service.go、updater.go | TestServiceRoleValidatedBeforeConfigurationRead、TestServiceCompositionIsolatesRuntimeResponsibilities 通过；显式角色、无 all、装配隔离 |
| REQ-SPLIT-003 | updater.go 复用原采集/调度 | 原 Go 行情与调度回归通过；范围、周期、限频未改 |
| REQ-SPLIT-004 | main.go 工作台装配 | Go 全量与前端 162 个测试通过；UI 无修改 |
| REQ-SPLIT-005/006 | api/updater_client.go、updater_router.go、market_refresh.go | 双路由刷新、Token、边界、故障映射、取消、重定向、网络重放测试通过；现有进程取消测试通过 |
| REQ-SPLIT-007/008 | data.New / data.Open、main.go | 只连接无 DDL 单元测试通过；新增真实 MySQL 测试已编译，实际执行按用户决定豁免 |
| REQ-SPLIT-009 | Dockerfile、compose.nas.yaml、配置样例、Makefile | Compose 配置解析通过，linux/amd64 二进制编译通过；两镜像实际构建豁免 |
| REQ-SPLIT-010 | 本文件、运行手册 | 用户调整后的编译门槛通过，独立代码评审通过；真实部署未执行 |

## 实际执行

- 基线 `1975cbd06cb90b1f9e3fc4b363189f89668fd09a`，设计批准版本 `03abaed`。`git fetch github` 因 SSH publickey 认证失败，本次基于本地 main，未声明远端最新。
- `go build ./...`：通过。
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/trading-nas-updater-amd64 .`：通过；同一二进制通过必填角色分别运行两种服务。
- `npm --prefix web run build`：通过。
- Go 全量测试及有测试包覆盖率：通过，总计 86.7%；market 94.3%、indicator 91.2%、strategy 94.8%、backtest 90.4%。
- `npm --prefix web run check`：通过，20 个测试文件、162 个测试，语句覆盖率 91.81%。
- `go test . -run '^TestDocumentation' -count=1`、`git diff --check`：通过。
- `go vet ./...`：通过。
- `go test -tags=integration ./... -run '^$'`：仅编译通过，不作为集成验收。
- `go test -race ./api -run 'Test(RemoteRefresh|Updater)' -count=1`：通过。
- `UPDATER_BIND_IP=127.0.0.1 docker compose -f compose.nas.yaml config --quiet`：通过，只解析样例，不启动容器。
- `bash scripts/verify.sh --full`：前端、文档、Go 全量/覆盖率及核心覆盖率阶段通过；用户调整验收范围后在全量 Race 阶段停止，后续阶段未继续。因此不声明完整脚本通过。
- 两份现有配置的 MySQL compatibility 预检均超时；未创建隔离库或访问业务表。NAS 当前不可达已由用户确认。未执行镜像构建或真实 NAS 部署。

## 独立评审与修复

review_service_split 对角色边界、数据库初始化、HTTP、部署与清理进行只读评审，发现 1 个 P2：Go 默认 Transport 即使没有显式重试仍可能重放 POST。

已使用专用仅 HTTP/1 的 Transport，并清除 GetBody。新增真实网络测试先在旧实现失败，再在修复后通过：复用连接写入前失败不 redial/replay；TLS 服务支持 HTTP/2 时仍以 HTTP/1 调用。评审 Agent 独立重跑后确认 P2 关闭，未报告其他 P1/P2。

## 未验证范围

NAS 现场连通性、MySQL 真正的双连接运行、schema 初始化、新镜像构建和容器部署尚未验证，按用户明确调整不阻塞本次代码合并。后续联网部署按运行手册先备份、停旧单体、启动 updater，再连接 workbench。生产配置仍保留在用户本地，本次不覆盖。
