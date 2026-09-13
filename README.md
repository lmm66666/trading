# 股票数据与策略回测平台

Go 服务提供 A 股行情/财报采集、技术策略回测、持久化扫描快照和 HTTP 查询。接口见 [api/api.md](api/api.md)，架构约束见 [AGENTS.md](AGENTS.md)。

## 启动

要求 Go 1.25.7+、MySQL 5.7/8.0。创建 trading 数据库，复制 config.example.yaml 为 config.yaml 并配置数据库连接，再运行：

```bash
go mod download
go run . -config config.yaml
```

服务监听8080，初始化迁移旧业务表与新内核表。内核使用 UTC DATETIME(6)，自动装配 MySQL 队列、结果仓储、行情读取、策略 Registry、回测/扫描服务及固定 compute worker。EngineVersion 显式为 strategy-kernel-v1，二进制策略行为变化时需更新该版本。

## 持久化任务

- POST /api/v1/backtest-runs、POST /api/v1/scan-runs 创建任务，使用显式幂等键并返回202/Run ID。
- GET 任务路径查询状态，POST 子路径 /cancel 请求取消；订单/成交/权益使用 sequence 游标分页。
- /api/v1/signal-snapshots/latest 只读已发布快照，续页绑定 SnapshotID；/api/v1/strategies 提供版本和参数目录。
- 旧 /api/stocks/signal 只读快照并返回六位 codes；旧 /api/stocks/backtest 精确解析活跃证券、创建任务，最多同步等待2秒。

worker 不依附 HTTP 请求存活，响应结束或等待超时不取消任务。Ctrl+C/SIGTERM 或 worker/HTTP 故障取消根 context；等待 worker 和旧调度器退出后再关闭数据库。HTTP 配置读头、读写与空闲超时。

## 当前切换阶段

技术信号/回测路径已使用新内核，没有旧技术引擎生产调用。Task15 前仍保留旧行情采集、价格查询及财报/宏观接口，只启动旧行情调度；新行情采集与查询服务的运行时替换留 Task15，不同时启动两套采集。中间状态不应作为已完成迁移版本部署。

新计算依赖迁移完成的 COMPLETE 版本，旧表导入说明见 [MySQL README](internal/infrastructure/mysql/README.md)。导入不会推断主数据 active/lot_size，六位兼容接口只匹配已补齐并激活的证券；无完整行情或未知证券明确失败，不回退旧 K 线。

## 验证

```bash
go test ./...
go test -race ./api ./internal/application ./internal/infrastructure/mysql
go vet ./...
```

MySQL 集成测试使用 integration 标签和 Docker；SQL mock 只能验证 SQL/映射，不能替代真实事务与并发验证。
