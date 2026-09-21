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

## 2. 本地启动

创建 `trading` 数据库后：

```bash
cp config.example.yaml config.yaml
# 编辑本地 config.yaml；该文件不得提交或进入镜像
go mod download
npm --prefix web ci
npm --prefix web run build
go run . -config config.yaml
```

服务默认监听 `:8080`，同源提供行情工作台和 API。前端开发模式：

```bash
npm --prefix web run dev
```

Vite 将 `/api` 代理到 `:8080`。

本地库没有行情数据时，可先灌注固定种子的演示行情（3 只股票与 1 个期货主力连续各 400 根日线）：

```bash
go run ./cmd/mock-data -config config.yaml.local
```

`config.yaml.local` 指向本地测试 MySQL，同样不得提交；数据生成与发布规则见 [mock 灌注设计](design/cmd/mock-data.md)。

## 3. 启动与停机语义

- 启动时只迁移证券主数据和新策略内核表；不创建、不变更、不写入旧技术 K 线表。
- `main.go` 启动 HTTP、持久化任务 Worker、股票行情调度和可选期货调度。
- 停机先取消根 context，等待 HTTP、Worker 和调度 goroutine 退出，最后关闭数据库。
- 已持久化但未完成的任务不会因某个 HTTP 请求断开而消失；服务停机中断由租约和后续接管处理。

## 4. 配置边界

- `Worker` 配置任务 Worker 数、租约、轮询、兼容接口同步等待，以及扫描/行情刷新的有界并发。
- `Market.StockRequestIntervalSeconds` 控制新浪行情共享限频，默认且不得低于 5 秒。
- `Market.FuturesEnabled` 开启固定期货主力连续日线；`Market.FuturesRefreshIntervalHours` 控制刷新周期。
- 期货来源按配置历史起点重读完整快照；股票按最近 20 根日线重叠增量刷新。
- 本地配置、密码、Token、数据库转储和导出包不得提交或写入镜像层。

具体模块语义见 [应用层设计](design/internal/application.md) 和 [Broker 设计](design/pkg/broker.md)。

## 5. Docker

构建 amd64 镜像：

```bash
docker buildx build --platform linux/amd64 -t trading:latest --load .
```

以只读方式挂载配置：

```bash
docker run -d --name trading -p 8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  trading:latest
```

也可以覆盖容器命令指定其他容器内路径：

```bash
docker run -d --name trading -p 8080:8080 \
  -v "$PWD/config.yaml:/run/secrets/trading.yaml:ro" \
  trading:latest -config /run/secrets/trading.yaml
```

镜像进程以非 root 用户运行；配置只在运行时挂载。

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
