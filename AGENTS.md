# 项目概述

本项目是 Go 编写的 A 股行情、财报、指标、策略扫描与回测平台。技术策略直接用 Go 实现，运行时使用版本化行情、持久化任务和不可变扫描快照；长期方向是逐步提供类似 TradingView 的指标、脚本和回测能力。

## 当前架构

```text
trading/
├── main.go                         # 组合根：HTTP、Worker、行情调度与优雅停机
├── config/                         # DB 与 Worker 配置
├── api/                            # Gin API、V1 持久化任务和兼容适配器
├── business/                       # 财报与宏观业务；不再承载技术策略
├── data/                           # 财报/旧表迁移回滚 Repository
├── model/                          # 财报及迁移兼容模型
├── internal/
│   ├── market/                     # 行情领域值、Bar、Dataset、复权与对齐
│   ├── indicator/                  # 带有效位的指标图
│   ├── strategy/                   # 策略接口、时间线、Registry 与内置 Go 策略
│   ├── backtest/                   # 账户、撮合、成本、回测与指标
│   ├── application/                # 行情采集/查询/调度、回测/扫描、持久化 Worker
│   ├── financialscreen/            # 财报筛选器
│   ├── port/                       # 应用端口与 DTO
│   └── infrastructure/mysql/       # 版本化行情、任务、结果和快照适配器
├── pkg/broker/                     # 新浪财报/宏观与东方财富版本化行情适配器
├── cmd/migrate-strategy-kernel/    # 旧行情 dry-run、迁移、检查点与重跑命令
├── scripts/verify.sh               # 覆盖率、Race、集成、性能和镜像门禁
└── shell/                          # 旧批量运维脚本
```

旧 `pkg/filter`、`pkg/strategy`、`business.StockDataService` 和旧行情调度器已删除。旧 K 线 Repository/Model 只供迁移或回滚工具使用，生产服务不读取、不写入，也不在正常启动时执行其 `AutoMigrate`。

## 运行约束

- Go 1.25.7+；MySQL 5.7 或 8.0。
- 时间戳统一使用 UTC `time.Time` / `DATETIME(6)`；交易日期使用独立 date-only 值。
- 新计算只读取大于 0 的 `COMPLETE` 行情版本，任务执行期间不得重新解析最新版本。
- Price/Money 使用缩放 10000 的有符号整数；版本和序号使用无符号整数。
- 同一策略行为变更必须新增策略版本；撮合、成本或引擎语义变化必须提升 `EngineVersion`。
- 信号在当前 Bar 收盘后产生，订单最早在下一根可交易 Bar 开盘成交，禁止未来数据。
- 全市场扫描必须通过 `BatchDatasets` 有界批量加载，不得恢复逐证券 Repository 查询。
- 不透明身份字段按 UTF-8 字节限制并精确区分大小写和尾空格。
- Docker 镜像不得包含本地配置、凭据、数据库转储或导出包；运行时只读挂载配置。

## 开发与测试

- 遵循 Go 标准编码规范，新增业务逻辑配套单元测试。
- 总测试覆盖率保持 80% 以上；`internal/market`、`indicator`、`strategy/...`、`backtest` 各自保持 90% 以上。
- 快速检查：`go test ./... && go vet ./...`。
- 完整交付检查：`bash scripts/verify.sh`。
- MySQL 集成测试需要 Docker；环境不可用时必须明确报告，不能用 SQL mock 或仅编译代替真实 MySQL 5.7/8.0 验证。

## 迁移与接口

- 先运行 `go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true`，确认后再以 `-dry-run=false` 写入新表。
- `/api/v1` 使用显式策略版本、幂等键、持久化 Run 与 SnapshotID 游标。
- 兼容 `signal` 只读最新已发布快照；兼容 `backtest` 创建持久化任务；兼容历史保存、增量刷新和价格查询均使用版本化行情服务。
- API 请求限制、状态码、固定执行假设和分页语义以 `api/api.md` 为准。
