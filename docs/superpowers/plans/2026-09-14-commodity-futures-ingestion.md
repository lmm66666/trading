# 新浪商品期货主力日线实施计划

> 历史计划（已废止）：MySQL 验收方式已由 [REQ-2026-002](../../requirements/active/REQ-2026-002-mysql8-cross-architecture.md)与[MySQL 设计](../../../internal/infrastructure/mysql/DESIGN.md)取代，本文仅保留需求追溯价值。

**目标：** 使用一个已实测可用的新浪期货日线接口，为 AU、AG、FU、SC、LU、J、JM、ZC 八个主力连续品种保存日线与本地周线。

**架构：** `SinaFuturesSource` 按主力符号请求完整历史，复用股票链路同一个 5 秒全局限频器；`MarketIngestionService` 复用现有合并、周线聚合和版本发布逻辑；独立的固定品种 scheduler 每日刷新；统一行情查询 API 按规范 Instrument ID 读取版本化 Bar。

**范围约束：**

- 只接新浪 `InnerFuturesNewService.getDailyKLine`，不引入 Python sidecar，不分别对接四家交易所。
- 只保存新浪已生成的主力连续 OHLCV；不采集真实交割合约，不自行选择主力或计算换月因子。
- 首批固定为 `AU0/AG0/FU0/SC0/LU0/J0/JM0/ZC0`，不接受调用方传 URL 或任意符号。
- 新浪期货与股票请求共用一个 `rate.Limiter`，默认且不得低于每 5 秒一次，重试同样消耗 token。
- 继续使用 `t_instruments`、`t_market_data_versions`、`t_market_bars`；仅扩展 Instrument code 长度以容纳 `.MAIN`，允许停机迁移，不增加期货专用表或未使用的合约元数据字段。
- 新浪响应缺少通用成交额，`Bar.Amount` 保存为 0；持仓量与结算价本期只做响应合法性校验，不持久化。
- 总覆盖率保持 80% 以上，新增解析、调度和 API 分支用单元测试覆盖。

## Task 1：期货连续 Instrument

- [x] 增加 SHFE、INE、DCE、CZCE 与 `.MAIN` ID 校验。
- [x] 扩展 Instrument code 长度以保存 `.MAIN`，资产类型与品种由领域身份确定。
- [x] 保持股票六位代码兼容。

## Task 2：新浪期货 adapter

- [x] 写冻结 JSONP fixture 和失败测试。
- [x] 实现固定符号映射、严格 JSONP/十进制解析、响应上限和取消传播。
- [x] 用单位因子适配现有日线 ingestion port。
- [x] 补充 `Retry-After`、响应体读取失败与尾随垃圾响应测试。

验证：

```bash
go test ./pkg/broker -run TestSinaFutures -count=1
```

## Task 3：调度与装配

- [x] 固定八个主力连续 Instrument，按稳定顺序逐一刷新。
- [x] 失败按品种隔离并保存汇总，取消时停止后续请求。
- [x] 配置开关与刷新周期，默认周期 24 小时。
- [x] 股票 scheduler 明确限制为 SSE、SZSE、BSE，避免误用股票源刷新期货。
- [x] 期货完整快照每次从历史起点重读，股票继续采用最近 20 根重叠窗口。

验证：

```bash
go test ./internal/application -run TestFuturesScheduler -count=1
go test . -run 'Test(ResolveMarket|KernelComposition|LoadConfig)' -count=1
```

## Task 4：统一行情查询

- [x] 增加 `GET /api/v1/market/bars`，使用完整 Instrument ID。
- [x] 支持 daily/weekly、raw/qfq、版本、日期和数量边界。
- [x] 严格拒绝重复与未知查询参数。
- [x] 更新 `api/api.md` 与 `AGENTS.md`。

## Task 5：验证与交付

- [x] 使用八个真实符号做只读在线 smoke test。
- [x] 运行 `go test ./...`、`go vet ./...` 和覆盖率门禁。
- [x] 历史执行 `bash scripts/verify.sh` 时，前六项通过；当时 Docker provider 不可用，旧数据库容器与镜像门禁未执行。该结果不代表现行 MySQL 8.4.x 门禁通过。
- [x] 代码审查并修正发现。
- [ ] 按项目 Git 规范提交、合并回 main、清理开发分支。
