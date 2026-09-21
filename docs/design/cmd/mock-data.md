---
status: approved
authority: normative
approval_provenance: session-decision
approved_by: user
approved_at: 2026-09-21
approved_revision: null
owns: ["cmd/mock-data/"]
related: []
---

# 本地 mock 行情灌注命令设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `cmd/mock-data` 本地测试行情灌注 |
| 最后更新 | 2026-09-21 |

## 1. 职责与非职责

本命令向本地 MySQL 灌入固定种子的演示行情：贵州茅台（SSE:600519）、五粮液（SZSE:000858）、宁德时代（SZSE:300750）与原油主力连续（INE:SC.MAIN）各 400 根日线及一条 1/1 复权因子，用于本地联调与前端验证。

本命令只服务本地测试库；生产环境的行情由调度器从外部来源采集，不使用本工具。它不提供自定义标的、根数或时间范围，也不执行删除或清理；重复运行依赖 Publish 的 digest 幂等（内容相同不产生新版本）。

## 2. 操作入口

```bash
go run ./cmd/mock-data -config config.yaml.local
```

`-config` 默认 `config.yaml.local`，是唯一支持的参数；配置文件不得提交。每个标的输出一行发布结果（标的、日线根数、数据版本）。

## 3. 依赖和边界

- 复用主程序配置格式（yaml 顶层 `Config` 包装）并校验 DB 字段完整性；错误消息不包含连接信息。
- 通过 `data.New` 建立连接并执行启动迁移（含版本锚点），随后关闭连接；进程结束即退出，不常驻。
- 发布走 MySQL adapter `MarketDataRepository.Publish` 的完整事务：自动创建 instrument、digest 幂等、分配新数据版本并置 `COMPLETE`。
- `Publish` 不写展示元数据，本命令在发布后以 UPDATE 补齐 `name`、`board`、`active`、`lot_size`；无匹配行视为失败。
- 依赖方向：`config`、`data`、`internal/infrastructure/mysql`、`internal/market`、`internal/port`；不依赖 API 层与 Worker。

## 4. 数据生成不变量

- 交易日锚点为 UTC 今日或之前最近的非周末日；向前取 400 个非周末交易日，UTC 午夜、升序。
- 每根 K 线 OpenTime 为交易日 01:30Z（09:30 CST）、CloseTime 为 07:00Z（15:00 CST），满足 UTC 微秒精度。
- 价格使用 mulberry32 种子随机游走：收盘 ±3%、开盘 ±1.2%、高低各扩展至多 1.2%，同一 seed 输出确定；下限为起始价 1/4 且不低于 1 元（`ValueScale` 缩放）。
- 成交量为 375000–1125000 的确定值；成交额按缩放换算为成交量×收盘价。
- 复权因子在首根 K 线收盘前 24 小时生效（1/1），保证 QFQ 视图可用。
- 批次经 `CanonicalMarketBatch` 校验：UTC 微秒时间、OHLC 与成交量合法、按 CloseTime 排序去重。

## 5. 测试与验收证据

单元测试覆盖标的清单（恰好 3 只股票 + 1 个期货）、交易日与锚点生成、批构造 canonical 校验、`run` 参数拒绝与发布流程输出。

```bash
go test ./cmd/mock-data
```

## 6. 相关文档

- [运行手册](../../operations.md)
- [MySQL 设计](../internal/infrastructure/mysql.md)
- [兼容数据访问设计](../data.md)
