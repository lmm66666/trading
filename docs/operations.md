# 运行手册

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 最后更新 | 2026-09-14 |
| 权威范围 | 本地启动、配置、Docker、维护窗口和运行限制 |

## 1. 环境要求

- Go 1.25.7+
- Node.js 24+
- MySQL 5.7 或 8.0
- 完整集成验证和镜像构建需要可用的 Docker daemon

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

## 3. 启动与停机语义

- 启动时只迁移财报、证券主数据和新策略内核表；不创建、不变更、不写入旧技术 K 线表。
- `main.go` 启动 HTTP、持久化任务 Worker、股票行情调度和可选期货调度。
- 停机先取消根 context，等待 HTTP、Worker 和调度 goroutine 退出，最后关闭数据库。
- 已持久化但未完成的任务不会因某个 HTTP 请求断开而消失；服务停机中断由租约和后续接管处理。

## 4. 配置边界

- `Worker` 配置任务 Worker 数、租约、轮询、兼容接口同步等待，以及扫描/行情刷新的有界并发。
- `Market.StockRequestIntervalSeconds` 控制新浪行情共享限频，默认且不得低于 5 秒。
- `Market.FuturesEnabled` 开启固定期货主力连续日线；`Market.FuturesRefreshIntervalHours` 控制刷新周期。
- 期货来源按配置历史起点重读完整快照；股票按最近 20 根日线重叠增量刷新。
- 本地配置、密码、Token、数据库转储和导出包不得提交或写入镜像层。

具体模块语义见 [应用层设计](../internal/application/DESIGN.md) 和 [Broker 设计](../pkg/broker/DESIGN.md)。

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
4. 运行数据校验和 MySQL 5.7/8.0 集成测试。
5. 启动新版本并观察任务与行情发布。

简单新增表/列可以由受控 AutoMigrate 完成；删除、重命名、索引/约束调整和数据重写使用显式迁移。破坏性变更的默认回滚方式是旧程序加数据库备份恢复。

旧行情内核迁移的完整流程见 [迁移命令设计](../cmd/migrate-strategy-kernel/DESIGN.md)。

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

完整交付：

```bash
bash scripts/verify.sh
```

完整脚本包含覆盖率、Race Detector、静态检查、5000 证券性能、MySQL 5.7/8.0 集成和不含本地配置的镜像构建。Docker 不可用时必须准确报告未完成的步骤，不能用 SQL mock 替代真实事务与并发验证。

## 9. 相关文档

- [系统设计](architecture/system-design.md)
- [Roadmap](roadmap.md)
- [API 契约](../api/api.md)
