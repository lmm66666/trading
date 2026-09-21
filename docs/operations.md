# 运行手册

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 最后更新 | 2026-09-14 |
| 权威范围 | 本地启动、配置、Docker、维护窗口和运行限制 |

## 1. 环境要求

- Go 1.25.7+
- Node.js 24+
- MySQL 8.4.x LTS
- 远端集成验证需要可达的获批 MySQL 服务；镜像构建需要可用的 Docker daemon

## 2. 两种服务与本地启动

服务角色必须通过 `-service updater|workbench` 指定，不再支持旧的单体命令或 all 模式。数据库初始化归 updater；workbench 只连接已经初始化的数据库。

先准备两份本地配置（不得提交）：

```bash
cp config.updater.example.yaml config.updater.yaml
cp config.example.yaml config.yaml
openssl rand -hex 32
```

将生成的随机 Token 填入两份配置的 `Config.Updater.Token`。正常查看全量数据时，两份配置的 `DB` 都指向 NAS 同一业务库；workbench 的 `Updater.URL` 指向 NAS 更新服务（如 `http://nas.local:8081`）。示例中的 Token 和数据库密码均需替换，不能当作正式凭据。沿用旧配置的 Market/Worker 数值；不要因为示例默认值而改变已有期货开关、更新间隔或限频。

本地调试时将两个服务指向本地 mock 库，URL 改为 `http://127.0.0.1:8081`；mock 库与 NAS 库独立，不自动同步或回退切换。updater 启动后先等待首个刷新周期；需要立即更新股票时，通过 workbench 的手动刷新接口触发。纯查看 mock 数据也可只启动 workbench，届时手动刷新会显示更新服务不可达。

```bash
go mod download
npm --prefix web ci
npm --prefix web run build
# 两个终端分别运行；首次初始化先启动 updater
make run-updater
make run-workbench
# 等价命令
# go run . -service updater -config config.updater.yaml
# go run . -service workbench -config config.yaml
```

workbench 默认监听 `127.0.0.1:8080`，同源提供页面与 API。前端开发仍可 `npm --prefix web run dev`，Vite 将 `/api` 代理到 `:8080`。

本地库没有行情数据时，可先灌注固定种子的演示行情（3 只股票与 1 个期货主力连续各 400 根日线）：

```bash
go run ./cmd/mock-data -config config.yaml.local
```

`config.yaml.local` 指向本地测试 MySQL，同样不得提交；数据生成与发布规则见 [mock 灌注设计](design/cmd/mock-data.md)。

## 3. 启动、断网与停机

- updater 先执行现有证券主数据/内核 schema 初始化，再运行内部刷新 HTTP 和股票/可选期货调度；不运行扫描/回测 Worker。不创建或变更旧技术 K 线表。
- workbench 不执行 DDL、不装配 Broker/调度，运行业务 HTTP、静态前端与持久化计算 Worker。启动不要求 updater 在线，但 MySQL 必须可达且已初始化。
- updater 不可达时，仍可使用已保存的行情与工作台功能；手动刷新返回 503。等待刷新超时为 504，服务认证或响应异常为 502。不会自动切到本地 mock 库。
- 刷新 POST 不自动重试。超时/断线不等于 NAS 未执行；全市场 202 只代表接受，已接受的任务不因工作台关闭而取消。单证券刷新沿请求 context 取消。
- 两服务分别取消根 context，等待自己拥有的 HTTP/后台工作退出，最后关闭自身数据库连接。电脑关闭不影响 NAS 更新；扫描/回测等电脑服务重启后按现有租约机制恢复。
- 每个目标库只部署一个 updater 和一个 workbench。updater 升级先停旧实例，不做新旧实例重叠的滚动更新；进程内刷新防重不支持多副本。

## 4. 配置边界

- `Server.ListenAddress`：updater 默认 `:8081`，workbench 默认 `127.0.0.1:8080`。容器中运行工作台时改为 `:8080`，宿主只映射回环地址。
- `Updater.Token`：两端相同、至少 32 字节非空白可打印 ASCII；使用随机生成值。`Updater.URL`：workbench 必填，只接受 http/https 源地址，不含用户名密码、路径、query 或 fragment。
- `Worker`：workbench 使用原任务并发、租约、轮询、同步等待和扫描批次；updater 只使用 `ScanBatchSize` 作为行情并发。
- `Market`：仅 updater 使用，股票与期货共享限频，默认且不得低于每 5 秒一次；期货开关与周期保留原配置。
- 股票启动满 24 小时后首次自动更新，再按原 ticker 更新，仍重抓最近 20 根日线；期货启动后等待配置的刷新间隔，沿用固定八品种和全历史刷新。重启重新计时。全市场手动刷新仍只触发股票调度，立即执行且不重置定时节拍。
- Token、密码、内网地址、完整配置、转储和导出包不得提交或进入镜像。服务调用 Token 不进入浏览器。

## 5. NAS Docker 部署

Dockerfile 有两个目标。NAS 只需 updater 镜像，不构建或携带前端：

```bash
make image-updater
# 若需要容器化工作台，另行构建
make image-workbench
```

镜像按 linux/amd64 构建、以非 root 运行。NAS 已有 MySQL，Compose 不新建 MySQL 容器或数据库卷。

部署前备份业务库，停止旧单体，将 `config.updater.yaml` 放在 Compose 文件同目录；容器中的 DB.Host 使用可达的 NAS/MySQL 地址，不能误填容器自己的 127.0.0.1。配置文件需对容器 app 用户可读，只读挂载，不要设为所有人可写。

```bash
# 将 nas.local 解析到的局域网 IP 填入环境变量，不能填写公网 IP。
export UPDATER_BIND_IP='<NAS_LAN_IP>'
docker compose -f compose.nas.yaml config --quiet
docker compose -f compose.nas.yaml up -d --build updater
docker compose -f compose.nas.yaml logs --tail=50 updater
```

Compose 在变量未设置或本地配置文件不存在时拒绝启动，不自动创建空配置目录。端口只绑定选定 NAS 局域网接口；NAS 防火墙限定 MySQL 与 8081 的所需访问来源。本次不开放公网。未来外网接入需另行配置安全网络通道或经过设计的网关，不直接映射数据库端口。

updater 初始化成功后，在电脑配置 NAS DB/Updater.URL，运行 `make run-workbench`。可选工作台容器：

```bash
# 配置中 Server.ListenAddress 使用 :8080
docker run --rm --name trading-workbench -p 127.0.0.1:8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" trading-workbench:latest
```

切换验收：图表读取已有 COMPLETE 版本；手动刷新走 NAS；扫描/回测和工作台写入正常；关闭电脑服务不影响 updater；停 updater 时工作台仍可读数据而刷新明确失败。首次初始化尚无 COMPLETE 数据时沿用无行情语义，不把 schema 创建成功当成行情已准备好。

回滚时先停止两个新服务，再启动旧发布版本单体和旧配置。本次不改变数据格式或表结构，保持业务数据；若同时做了其他迁移，按对应迁移的恢复要求处理。新版本本身不保留 all 模式。

## 6. 数据库变更

项目允许停机更新。涉及表结构或数据语义变化时，默认流程：

1. 停止服务和行情/扫描写入。
2. 备份数据库并在副本演练。
3. 执行迁移。
4. 运行数据校验和 MySQL 8.4 集成测试。
5. 启动新版本并观察任务与行情发布。

简单新增表/列可以由受控 AutoMigrate 完成；删除、重命名、索引/约束调整和数据重写使用显式迁移。破坏性变更的默认回滚方式是旧程序加数据库备份恢复。

旧行情内核迁移的完整流程见 [迁移命令设计](design/cmd/migrate-strategy-kernel.md)。

## 7. 旧行情迁移入口

先只读检查：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true -batch-size=1000
```

确认报告无缺失后，在维护窗口写入：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size=1000
```

迁移不会推断证券 active 和 lot_size；上线前必须补齐并激活 `t_instruments`。六位代码兼容接口只匹配活跃证券。

## 8. 验证

快速检查：

```bash
npm --prefix web run check
go test ./...
go vet ./...
```

按风险选择门禁：

```bash
bash scripts/verify.sh                 # 默认本地门禁，无 MySQL / Docker 调用
bash scripts/verify.sh --mysql         # 追加远端隔离数据库验收
bash scripts/verify.sh --image         # 追加 linux/amd64 镜像构建
bash scripts/verify.sh --full          # 所有门禁；等价于 --mysql --image
bash scripts/verify.sh --help          # 仅显示用法
```

默认包含前端构建/覆盖率、Go 测试/覆盖率、文档、Race Detector、静态检查、5000 证券性能及容器配置安全检查。SQL/模型/索引/迁移/事务/锁/持久化队列/数据库驱动变化必须选择 MySQL；Dockerfile/构建依赖/打包/部署方式变化必须选择镜像；完整触发规则以 [工程标准](standards/engineering.md) 为准。CI/发布可用 `--full`。

未选择的外部门禁会显式显示“未选择”，不能当作通过；评审确认不适用时在单项记录 `not-required` 和原因。必要门禁缺少前提或执行失败则记录阻塞/失败，禁止归档与合并。脚本未知参数立即退出，所选门禁任一失败返回非零状态。

本地不得启动或拉取 MySQL 作为验收环境。远端预检只读取版本和编译架构；完整集成测试为每个测试创建 `trading_test_` 前缀的随机空数据库，结束后删除。测试直接读取仓库根目录下、本地保存且已被 Git 忽略的 `config.yaml` 中的数据库配置；配置内的业务库名只用于定位连接，测试连接必须改写到随机空数据库。缺少文件、配置无效或连接失败时门禁失败。凭据、完整 DSN 和内网地址不得写入仓库、日志或命令示例。

Docker Hub 或镜像构建阶段需要代理时，宿主侧继续使用标准 `http_proxy`、`https_proxy`；构建容器内的 npm、apk 和 Go 下载可额外通过 `TRADING_DOCKER_BUILD_PROXY` 注入容器可访问的代理 URL。Docker Desktop 中宿主回环代理应使用 `host.docker.internal`，不能在容器内继续使用 `127.0.0.1`。验证脚本只把该值作为 Docker 预定义代理构建参数传入，不写入最终镜像环境；不得把个人代理地址提交到仓库。

测试进程被强制终止可能留下随机数据库。人工清理前必须确认没有活跃测试，并只处理已确认属于本项目测试且不再使用的 `trading_test_` 数据库。

## 9. 相关文档

- [系统设计](architecture/system-design.md)
- [Roadmap](roadmap.md)
- [API 契约](standards/http-api.md)
