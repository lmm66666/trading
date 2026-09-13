# 股票数据与策略回测平台

这是一个 Go 编写的 A 股行情、财报、指标、策略扫描与回测内核。当前技术路线以 TradingView 的使用体验为长期方向：先提供可复现的内置 Go 策略、指标和批量回测，再逐步增加受控脚本能力。接口详见 [API 文档](api/api.md)。

## 本地启动

要求 Go 1.25.7+、MySQL 5.7 或 8.0。创建 `trading` 数据库后：

```bash
cp config.example.yaml config.yaml
# 编辑本地 config.yaml；该文件不会进入镜像
go mod download
go run . -config config.yaml
```

服务默认监听 `:8080`。启动时只迁移财报、证券主数据和新策略内核表，不再创建或写入旧技术 K 线表。`Worker` 配置分别控制持久化任务 Worker 数、租约、轮询、兼容接口同步等待时间，以及扫描/行情刷新的有界并发数。

## 先迁移数据，再切换流量

新计算只读取大于 0 的 `COMPLETE` 数据版本。先执行只读检查：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true -batch-size=1000
```

确认报告无缺失后，在维护窗口写入新表：

```bash
go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size=1000
```

迁移支持检查点和幂等重跑，不会推断证券的 `active` 与 `lot_size`。上线前需补齐并激活 `t_instruments`；六位代码兼容接口只精确匹配活跃证券。旧仓储代码仅保留给回滚/迁移工具，生产路径不会回退旧 K 线。

## 策略与任务模型

- 内置策略版本 1：`daily_b1_buy`、`weekly_b1_buy`、`bottom_surge_pullback`。`EngineVersion` 为 `strategy-kernel-v1`；策略行为变化必须新增策略版本或提升引擎版本。
- 信号严格按时间线重放。第 N 根 Bar 产生的订单最早在第 N+1 根可交易 Bar 开盘成交，指标预热期返回无效值；策略不可读取未来数据。
- 这修复了旧实现可能使用未来数据的问题，因此同一历史区间的信号和收益可能与旧结果不同，这是预期的行为变化。
- 回测/扫描任务持久化在 MySQL，HTTP 请求结束或同步等待超时不会取消任务。任务固定行情版本、证券集合、策略版本、参数、执行配置和引擎版本，可重复复现。
- 扫描一次批量读取最多 5000 只证券，发布不可变快照；单只证券失败可得到 `PARTIAL_SUCCEEDED`，不会丢弃其他成功结果。

`POST /api/v1/backtest-runs` 与 `POST /api/v1/scan-runs` 创建任务；状态、取消、成交、权益与最新扫描快照接口见 [api/api.md](api/api.md)。旧 `/api/stocks/*` 路径保留为兼容适配器，但行情保存、增量刷新、价格查询、信号与回测均已切到新内核。

## Docker

镜像不包含任何本地配置，进程以非 root 用户运行。必须只读挂载配置：

```bash
docker buildx build --platform linux/amd64 -t trading:latest --load .
docker run -d --name trading -p 8080:8080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  trading:latest
```

也可以覆盖容器命令指定另一个容器内路径：`trading:latest -config /run/secrets/trading.yaml`。

## Redis / Kafka 取舍

第一阶段保持 MySQL-only：任务、租约、幂等、快照和行情版本在一个事务边界内，5000 证券扫描已使用批量读取和固定 Worker，部署更简单。以下指标出现后再引入额外组件：

- MySQL 队列领取或续租 P95 持续超过 100ms、待执行任务积压超过一个扫描周期：优先把任务队列迁到 Redis Streams；MySQL 仍保存最终结果和审计记录。
- 行情需要多个独立消费者、跨服务重放或峰值吞吐达到单库写入瓶颈：再以 Kafka 作为行情事件日志；数据库发布仍须保持数据版本原子性。
- 不建议仅为“逐股查询慢”引入 Redis/Kafka；该瓶颈已经通过一次批量加载全市场数据解决，缓存或消息系统不能替代查询模型和索引优化。

## 验证

```bash
bash scripts/verify.sh
```

脚本执行总覆盖率与核心领域覆盖率门禁、全量测试、Race Detector、`go vet`、5000 证券性能测试、MySQL 5.7/8.0 集成测试及无本地配置的镜像构建。MySQL 集成测试与镜像构建需要可用的 Docker daemon；SQL mock 不能替代真实事务与并发验证。
