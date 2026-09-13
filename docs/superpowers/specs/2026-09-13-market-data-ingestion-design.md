# 新浪股票与商品期货日线接入设计

## 1. 背景

项目已经完成策略与回测内核切换。股票行情不再经过旧 `business.StockDataService`、旧调度器和旧 K 线表，现有 `main` 使用东方财富 `MarketSource` 进入统一行情应用服务并发布到版本化行情表。

东方财富 `push2his.eastmoney.com` 当前会在低频单请求下直接断开连接。相同环境访问 `push2.eastmoney.com` 和 `datacenter-web.eastmoney.com` 正常，补充 `User-Agent`、`Referer` 和常见 `ut` 参数也不能恢复。该现象与 AKShare 社区近期记录的 `RemoteDisconnected` 一致，更接近特定域名或出口 IP 的连接级拒绝，而不是有明确窗口和 `429 Retry-After` 的普通频率限制。现有实现又会以 8 个 worker 并发，每只股票至少放大成 5 个请求，因此即使连接恢复，也容易再次触发上游防护。

本次把股票行情主源切回新浪。新浪已实测需要全局每 5 秒最多 1 次请求；该节拍作为硬配置，不再用并发 semaphore 代替频率限制。股票只从上游获取日线和前复权因子，周线在 Go 内由日线聚合。东方财富不作为自动 fallback，避免失败后放大流量和掩盖数据来源。

同时增加国内商品期货日线，首批覆盖黄金、白银、原油、燃料油、焦炭、焦煤和动力煤。期货数据来自 AKShare 封装的交易所公开数据；Go 服务继续负责领域校验、版本发布、查询、主力合约选择和连续序列计算。

这是个人项目，允许停机更新。设计优先复用当前内核和数据库，不建设双写、灰度切换、影子表、跨进程采集集群或长期兼容层。

## 2. 当前架构带来的调整

此前设计与当前 `main` 的差异按以下方式收敛：

- 保留当前版本化行情内核，但把生产装配从 `EastmoneyMarketSource` 改为新的 `SinaMarketSource`。
- 股票上游只抓日线和前复权因子；周线由应用层按实际交易日聚合，不再额外请求新浪周线。
- 股票请求使用真正的全局时间节拍器，固定最小间隔 5 秒、burst 为 1；worker 数只控制本地处理，不改变外部请求速率。
- 不再新建平行的 `internal/futuresdata/application/domain/infrastructure` 栈。
- 期货领域值放入 `internal/market`，用例放入 `internal/application`，接口放入 `internal/port`，MySQL 实现放入现有 `internal/infrastructure/mysql`。
- AKShare Go 客户端属于外部数据源 adapter，放入 `pkg/broker`；Python 服务放入 `sidecar/akshare-futures`。
- 继续使用现有 `t_instruments`、`t_market_data_versions`、`t_market_bars` 和 `COMPLETE` 版本读取语义。
- 数据库升级采用一次停机迁移，不再设计在线多阶段切换流程。

## 3. 目标

1. 用新浪日线和前复权因子替代不稳定的东方财富股票历史行情端点。
2. 接入 SHFE、INE、DCE、CZCE 的指定商品期货日线。
3. 保存真实合约 OHLCV、结算价、前结算价、成交量、持仓量和成交额。
4. 生成无未来数据的主力合约映射、原始主力序列和比例连续序列。
5. 让现有指标、扫描和行情查询读取股票与期货日线，不恢复旧行情表。
6. 每条数据可追溯到来源、原始代码和行情版本。
7. 保持总体测试覆盖率大于 80%，行情领域计算覆盖率大于 90%。

## 4. 非目标

- 不使用 Tushare，也不把东方财富作为股票行情自动 fallback。
- 不把新浪的分钟线、实时行情或单独周线接口接入版本化行情。
- 不采集分钟线、Tick、盘口或实时行情。
- 不接入境外期货、国际现货或场外报价。
- 不实现期货下单、保证金、交割、夜盘逐笔撮合和期货回测执行模型。
- 不允许 Go 动态调用任意 AKShare 函数，也不允许请求方传入 URL、Python 代码或文件路径。
- 不建设分布式采集、跨进程租约、Kafka、Redis 或独立数据平台。
- 不承诺交易所公开页面或 AKShare 具备商业 SLA；异常数据必须拒绝发布。

## 5. 目标架构

```text
股票行情（调整数据源，复用内核）
新浪日线 + qfq 因子 -> SinaMarketSource -> MarketIngestionService
                      -> 本地周线聚合 -> MarketDataRepository
                      -> 版本化股票行情

期货行情（新增）
交易所公开数据 -> AKShare -> Python sidecar
                              |
                              v
                    AkshareFuturesSource
                              |
                              v
                   FuturesIngestionService
                              |
                              v
                    FuturesRepository
           复用版本分配并原子写入通用 Bar 与期货字段
                              |
                              v
              MarketDataRepository / FuturesDataReader
                              |
                              v
                    指标、扫描、查询数据集
```

依赖方向遵循项目现有规范：

```text
api / pkg / infrastructure -> application -> port / market
```

`internal/market` 不依赖 Python、AKShare、HTTP、GORM 或 MySQL。`main.go` 只装配 sidecar client、repository、application service 和 scheduler。

## 6. 模块设计

```text
internal/market/
  instrument.go                    扩展交易所和期货代码校验
  futures_daily.go                 期货专有日线值对象
  futures_main.go                  主力状态、选择规则和连续因子

internal/port/
  futures_source.go                按交易所/日期获取期货日线
  futures_data.go                  期货专有数据读写接口

internal/application/
  futures_ingestion_service.go     拉取、规范化、校验和发布
  futures_scheduler.go             单进程日常调度
  futures_query_service.go         真实/主力/连续日线查询

internal/infrastructure/mysql/
  futures_daily_model.go
  futures_main_mapping_model.go
  futures_repository.go

pkg/broker/
  sina_market.go                  新浪日线与 qfq 因子 adapter
  akshare_futures.go               Go sidecar HTTP adapter

sidecar/akshare-futures/
  app/                             固定协议的 Python 服务
  tests/                           交易所 fixture 与协议测试

cmd/futures-sync/
  main.go                          历史回补和人工重跑
```

不创建新的通用 repository、版本系统或任务框架。只有期货专有字段和按交易所分区拉取方式使用新 port。

应用层每个真实合约只调用一次 `FuturesDataWriter.PublishContract`。MySQL `FuturesRepository` 在同一事务中复用现有版本分配和 Bar 修订 helper，再写入期货专有字段；应用层不能分别调用两个 repository 拼接事务。

## 7. 股票行情边界

### 7.1 新浪协议

股票原始日线使用：

```text
https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/
CN_MarketData.getKLineData?symbol=sh600000&scale=240&ma=no&datalen=N
```

前复权因子使用：

```text
https://finance.sina.com.cn/realstock/company/sh600000/qfq.js
```

原始日线响应只接受 `day/open/high/low/close/volume`。`datalen` 首次同步请求 10000，增量同步按回看窗口计算并设置下限；实测 `sh600000` 能返回 6388 根日线。响应为空、日期倒序重复、非法 OHLC、零价格或超出请求窗口均按坏数据处理，不发布空版本。

`qfq.js` 的因子 `f` 表示前复权价格为 `raw / f`。Go 使用十进制字符串经 `big.Rat` 转换为约分后的 `Numerator/Denominator`，禁止经过 `float64` 后再生成因子。若最早日线早于最早因子时点，在最早日线日期补一条取最早已知值的因子，保证整个数据集有定义。

### 7.2 请求节拍

- 所有新浪股票行情请求共享一个 `rate.Limiter`：`rate.Every(5*time.Second)`、burst 为 1。
- 日线和 qfq 因子都消耗一个 token；重试也必须重新取 token，不能在 adapter 内绕过全局节拍。
- 每只股票正常同步 2 次外部请求。5000 只股票的理论下限约 13.9 小时，这是选择免费新浪源后接受的运行约束。
- worker 可以并发加载数据库、校验和发布，但不能提高新浪出口速率。
- 遇到 `429` 或明确 `Retry-After` 时取较大等待值；连接重置使用有界指数退避，但每次重试仍遵守 5 秒节拍。

### 7.3 应用与发布

活跃股票源接口收敛为“日线 + 调整因子”，不再要求上游提供周线和公司行动：

```go
type EquityDailySource interface {
    FetchDailyBars(context.Context, market.InstrumentID, time.Time, time.Time) ([]market.Bar, error)
    FetchAdjustmentFactors(context.Context, market.InstrumentID) ([]market.AdjustmentFactor, error)
}
```

`MarketIngestionService` 合并已存和新增日线后，按 ISO 周和实际最后交易日生成周线：首日 open、最高 high、最低 low、末日 close、volume 求和，周 Bar 的时间取该周最后一根日线。未结束的最新周可以发布并在下一次同步原位修订；任何周线必须能映射到同版本日线收盘。

每次刷新把 Raw 日线、本地周线和完整 qfq 因子放入同一个 `MarketWriteBatch`，继续由 `MarketDataRepository` 原子发布 `PENDING -> COMPLETE` 版本。新浪不提供本设计所需的结构化公司行动，因此新版本不新增 `CorporateAction`；现存历史行动保留，但策略和价格查询只依赖调整因子。

查询、扫描和回测仍固定读取一个大于 0 的 `COMPLETE` 数据版本。东方财富 adapter 暂时保留用于已有测试和人工诊断，但不在 `main.go` 装配，也不自动回退。

### 7.4 东方财富诊断结论

本次低频探针结果：

| 探针 | 结果 |
|---|---|
| `push2his`，现有参数 | 首次请求即 empty reply / HTTP 000 |
| 增加 `User-Agent`、`Referer` | 仍为 empty reply |
| 增加 AKShare 常用 `ut` | 仍为 empty reply |
| 绕过本地代理直连 | 仍为 empty reply |
| 同环境 `push2`、`datacenter-web` | HTTP 200 |

因此不能把当前故障归结为“低于某个每秒请求数即可恢复”。更合理的判断是：`push2his` 存在按域名、出口 IP 或信誉状态执行的连接级拒绝，历史高并发可能是触发因素，但被拒后单次低频请求也不会立即恢复。当前代码的 8 worker 并发、每股最少 5 请求会显著放大风险；仅增加 header 或普通 sleep 不能形成可靠主源。

## 8. 首批期货范围

| 交易所 | 品种 | 含义 |
|---|---|---|
| SHFE | AU、AG、FU | 黄金、白银、燃料油 |
| INE | SC、LU | 原油、低硫燃料油 |
| DCE | J、JM | 焦炭、焦煤 |
| CZCE | ZC | 动力煤 |

交易所和品种均来自服务端配置白名单。未配置的交易所、品种或合成代码直接拒绝，不转发给 sidecar。

## 9. AKShare sidecar

### 9.1 选择 sidecar

AKShare 已处理多个交易所公开数据格式。直接把这些解析逻辑移植到 Go 会复制易变实现，因此 Python 只负责“调用固定 AKShare 函数并转换为稳定 JSON”；所有业务判断仍在 Go。

sidecar 固定使用 Python 3.12 和 `akshare==1.18.94`。升级 AKShare 必须先更新 fixture 并通过契约测试，不能在容器启动时自动升级。

### 9.2 请求

```http
POST /v1/futures/daily
Content-Type: application/json
```

```json
{
  "request_id": "01J...",
  "exchange": "SHFE",
  "trade_date": "2026-09-11",
  "products": ["AU", "AG", "FU"]
}
```

一次请求只包含一个交易所和一个交易日。sidecar 内部只能映射到 `get_futures_daily`，不能接受函数名。

### 9.3 响应

```json
{
  "schema_version": 1,
  "provider": "akshare",
  "provider_version": "1.18.94",
  "exchange": "SHFE",
  "trade_date": "2026-09-11",
  "rows": [
    {
      "raw_symbol": "au2612",
      "product": "AU",
      "open": "612.34",
      "high": "618.20",
      "low": "610.12",
      "close": "616.88",
      "settlement": "615.42",
      "pre_settlement": "611.20",
      "volume_lots": "123456",
      "open_interest_lots": "198765",
      "turnover_10k_cny": "987654.32"
    }
  ]
}
```

价格和数量使用十进制字符串，Go 侧转换为定点整数。sidecar 不连接数据库、不生成主力合约、不做连续复权。

### 9.4 空数据和错误

sidecar 必须区分：

- 已确认休市或品种尚未上市：成功响应，`rows=[]` 并返回明确 `empty_reason`。
- AKShare 意外返回空 DataFrame：`502 UPSTREAM_UNEXPECTED_EMPTY`。
- 上游 HTML、字段变化或无法解析：`502 UPSTREAM_BAD_PAYLOAD`。
- 网络超时：`504 UPSTREAM_TIMEOUT`。
- 参数或白名单错误：`400 INVALID_ARGUMENT`。

错误响应只包含 `request_id`、稳定错误码和 `retryable`，不返回上游 HTML、Cookie 或堆栈。

## 10. 领域模型

### 10.1 Instrument

现有 Instrument 扩展以下概念：

- `AssetClass`: `EQUITY`、`FUTURE`。
- `InstrumentKind`: `SPOT_EQUITY`、`FUTURE_CONTRACT`、`FUTURE_CONTINUOUS`。
- `Exchange`: 增加 `SHFE`、`INE`、`DCE`、`CZCE`。

真实合约使用完整年月：

```text
SHFE:AU202612
INE:SC202612
CZCE:ZC202609
```

主力连续合约使用：

```text
SHFE:AU.MAIN
INE:SC.MAIN
```

股票仍必须是六位数字；期货代码使用独立校验器。原始交易所代码保存在期货专有记录中。郑商所三位年份代码必须结合交易日期解析为完整年份，并保留解析规则版本。

### 10.2 通用 Bar 与期货字段

通用 `market.Bar` 继续保存 OHLCV、时间、交易状态和版本。期货专有值对象额外包含：

- `PreSettlement`、`Settlement`；
- `VolumeLots`、`OpenInterestLots`；
- `Turnover`；
- `RawSymbol`、`Product`；
- `StatisticsBasis`；
- `ProviderVersion`。

空价格不能转成零。交易所返回的指数、统计或连续代码，例如 0、88、888、99，不得作为真实合约写入。

成交量和持仓量历史统计口径发生变化时保存 `StatisticsBasis`，不在接入层擅自换算。FU 等存在制度边界的品种不自动跨边界拼接连续序列。

## 11. 数据库设计

### 11.1 复用表

- `t_instruments`：同时保存股票、真实期货和连续期货。
- `t_market_data_versions`：沿用全局版本分配和 `COMPLETE` 状态。
- `t_market_bars`：保存真实合约和连续合约的通用 OHLCV。

`t_instruments` 停机迁移时直接扩展：

- `exchange` 长度从 8 扩到 16；
- `code` 长度从 6 扩到 32；
- 增加 `asset_class`、`instrument_kind`、`product_code`、`delivery_month`；
- 增加可空的 `last_trade_date`、`contract_multiplier` 和 `tick_size`；没有可靠来源时保持空值；
- 现有记录一次性回填为 `EQUITY`、`SPOT_EQUITY`；
- 重建 `(exchange, code)` 唯一索引后执行重复检查。

### 11.2 新增表

`t_futures_contract_daily` 保存与 `t_market_bars` 同版本的期货专有字段：

- `instrument_id`、`trade_date`；
- `pre_settlement`、`settlement`；
- `volume_lots`、`open_interest_lots`、`turnover`；
- `raw_symbol`、`statistics_basis`、`provider_version`；
- `valid_from_version`、`valid_to_version`。

唯一键包含 `instrument_id + trade_date + revision`，当前版本查询使用 `instrument_id + trade_date + valid_to_version` 索引。

`t_futures_main_mappings` 保存：

- 连续 Instrument、交易日期、真实合约 Instrument；
- 决策日期、规则版本、换月原因；
- 旧/新合约持仓量和成交量；
- 连续价格因子；
- `valid_from_version`、`valid_to_version`。

不新增 observation、quality issue、ingestion run、跨进程 lease 或 trading session 表。采集失败保留旧 `COMPLETE` 版本，并通过调度摘要和结构化日志暴露。

### 11.3 发布语义

sidecar 按交易所返回分区，Go 校验后按真实合约拆分。每个合约使用现有全局版本分配器，在一个事务中写入：

1. `PENDING` 数据版本；
2. 通用 `t_market_bars` 修订；
3. `t_futures_contract_daily` 修订；
4. 将版本更新为 `COMPLETE`。

同一合约任一步失败则全部回滚。一个交易所中部分合约失败不回滚已经成功发布的其他合约，但该品种当日不生成新的主力映射，避免在候选不完整时换月。

主力映射和连续 Bar 使用独立 `COMPLETE` 版本发布，并记录其依赖的真实合约最高版本。查询固定版本后不得重新解析 latest。

## 12. 主力合约规则

默认规则版本为 `main_oi_hysteresis_v1`。交易日 `d` 只使用 `<= d` 的数据做决策，结果从下一个实际观测交易日生效。

候选必须是同交易所、同品种的真实合约，交割月不早于当前月份，价格有效且 `volume_lots > 0`。排序顺序为：

1. 持仓量降序；
2. 成交量降序；
3. 交割月份升序；
4. 标准合约代码字典序。

换月条件：

- 常规：候选 OI 至少为当前主力的 110%，连续 2 个交易日。
- 快速：候选 OI 至少为当前主力的 125%，持续 1 个交易日。
- 当前主力最短保持 3 个交易日；快速条件不能绕过该限制。
- 不允许从较远交割月回滚到较近交割月。

以下情况可强制换月，但仍从下一交易日生效：

- 距可靠最后交易日不超过 5 个交易日；
- 缺少最后交易日元数据时，当前合约已经进入交割月；
- 当前合约停止出现或连续 2 日无有效成交；
- 当前持仓量为零且存在有效远月候选。

`last_trade_date` 只接受交易所资料或受控人工配置；没有可靠值时保持空并使用“进入交割月”的保守规则，不从行情缺失反推出精确日期。

不存在候选时不延用失效合约，返回 `NO_ELIGIBLE_MAIN` 或 `NO_LIQUID_MAIN`。

历史构建按日期正序执行。首次出现某品种的当天只做选择，下一交易日才产生第一条主力映射。

## 13. 连续价格

提供两种视图：

- `RAW_MAIN`：直接拼接主力真实合约，保留换月跳空。
- `FORWARD_RATIO`：默认指标视图，保持最早历史段不变，只调整新合约及后续价格。

换月决策日旧、新合约均有有效 close 时：

```text
ratio = close(new) / close(old)
new_factor = old_factor / ratio
adjusted_price = raw_price * segment_factor
```

同一因子用于 OHLC 和 settlement；成交量、持仓量不调整。若换月锚点缺失，`RAW_MAIN` 可继续，`FORWARD_RATIO` 从该点停止并返回 `NO_ROLL_FACTOR`，不得使用未来日期补因子。

主力映射和连续序列必须满足前缀一致性：

```text
Build(all)[:k] == Build(all[:k])
```

## 14. 采集、调度与查询

### 14.1 股票日常同步

- 延续全市场逐证券同步，但新浪请求全局串行，固定 5 秒最小间隔。
- 首次同步每只股票请求最多 10000 根原始日线和完整 qfq 因子；日常同步重抓最近 30 根日线并重新获取因子。
- 同一轮只允许一个股票全市场刷新；服务重启后允许从头重跑，已发布股票不回滚。
- 单只股票失败记录稳定错误码并继续其他股票；失败股票保留旧 `COMPLETE` 版本。
- 股票同步不和期货 sidecar 共用 limiter，两个上游互不阻塞。

### 14.2 期货日常同步

- 默认每天 18:30 执行一次。
- 单进程 scheduler 按配置交易所顺序调用 sidecar，并使用独立于股票行情的有界并发。
- 每次重抓最近 10 个已观测交易日；相同内容不创建新版本。
- 网络和可重试上游错误最多重试 3 次，使用带抖动的指数退避。
- 单个分区失败记录在本轮摘要，下一轮或人工命令重试，不引入持久化采集任务。
- 夜盘归属完全采用交易所数据中的 `trade_date`，不按请求时间重新解释。

### 14.3 期货历史回补

```text
futures-sync --from 2018-01-01 --to 2026-09-11
```

命令按日期正序执行，已存在且 digest 相同的合约跳过。失败时返回非零状态并打印最后成功日期；再次运行从指定日期安全重做即可，不额外建设 checkpoint 表。

### 14.4 查询

新增只读接口：

- `GET /api/v1/futures/bars`：真实合约、`RAW_MAIN` 或 `FORWARD_RATIO` 日线。
- `GET /api/v1/futures/main-mappings`：主力映射、决策日期和换月原因。

请求必须包含 Instrument、日期范围和价格视图，可选数据版本。版本为空时只解析一次 latest `COMPLETE`，随后固定该版本完成全部查询。

第一阶段不增加公网同步接口。同步只由内部 scheduler 或 CLI 触发。

## 15. 错误与日志

- 新浪 adapter 把 HTTP 状态、连接重置、超时、空响应和格式漂移映射为稳定错误，不记录完整 URL 或上游正文。
- 股票调度摘要包含总数、成功数、失败数、预计剩余时间和最近一次成功请求时间；不为每次 limiter 等待打印日志。
- Go adapter 把 sidecar 错误映射为稳定的参数错误、临时上游错误、超时和坏数据错误。
- 领域校验错误不可重试；网络超时和明确临时错误可重试。
- 失败发布保持旧版本可读，不写半条 Bar，也不把缺失价格写成零。
- 新期货链路使用注入的 `slog`/Telemetry，不在领域计算中直接日志。
- 日志至少包含 `component`、`operation`、`exchange`、`trade_date`、`request_id`、`attempt`、`duration_ms` 和稳定 `error_code`。
- 不记录上游正文、Cookie、完整配置或数据库连接信息；同一错误只在负责处理的边界记录一次。

## 16. 停机迁移

本次允许停机，执行流程固定为：

1. 合并代码并完成测试，但保持期货调度关闭。
2. 停止服务，备份当前数据库。
3. 执行 `t_instruments` 显式 ALTER 和新表迁移。
4. 校验股票 Instrument 数量、唯一键、字段回填和现有版本读取。
5. 用 3 只含历史除权事件的股票执行新浪 smoke sync，核对 Raw、qfq 和本地周线。
6. 启动新版本，先运行少量日期的 `futures-sync`。
7. 核对真实合约、主力映射和连续序列后启用日常调度。

不实施双写、影子表、在线回填和旧新 schema 长期兼容。失败时停止服务，恢复数据库备份并运行旧版本。

## 17. 测试

### 17.1 Go

- 新浪 symbol 映射、日线 JSON、qfq.js、十进制有理数和异常响应解析。
- 5 秒全局节拍、burst=1、重试重新取 token、取消等待和多 worker 不突破速率。
- 从日线生成周线，覆盖节假日短周、未结束周修订、重复日期和日周收盘一致性。
- 新浪切换后 Raw/qfq 查询与既有版本固定语义。
- Instrument 股票/期货分流校验、郑商所年份解析和指数代码过滤。
- sidecar 响应缺字段、坏数值、错误日期、重复合约和意外空集。
- 期货专有字段与通用 Bar 同版本事务发布。
- 常规、快速、强制换月，最短持有、防倒退和无候选。
- 连续比例因子、缺锚点、前缀一致性和 suffix poisoning。
- scheduler 有界并发、重试、取消和部分失败摘要。
- 查询固定版本，不在一次请求中漂移到新版本。

期货领域、主力和连续序列包覆盖率大于 90%；项目总体大于 80%。

### 17.2 Python

- SHFE、INE、DCE、CZCE 冻结响应 fixture。
- 空 DataFrame、HTML 错页、字段变化、超时和白名单错误。
- JSON schema、数值字符串和 AKShare 版本输出。

### 17.3 集成

- 默认测试使用 fixture 和 `httptest`，不访问公网。
- MySQL 5.7、8.0 验证 Instrument 停机迁移、事务回滚、版本读取和索引。
- 完整交付运行 `bash scripts/verify.sh`。
- 在线 smoke test 只作为人工或夜间检查，不作为普通单元测试门禁。

## 18. 实施顺序

1. 实现新浪日线/qfq adapter、全局 5 秒节拍和 parser 测试。
2. 把股票应用服务改为日线输入、本地周线聚合，并切换 `main.go` 装配。
3. 扩展 Instrument、数据库字段和迁移校验。
4. 实现期货领域值、source/writer port 和单元测试。
5. 实现固定版本 AKShare sidecar 及 fixture 测试。
6. 实现 Go adapter、真实合约发布和 MySQL 集成测试。
7. 实现主力映射与连续序列。
8. 实现 scheduler、CLI 和只读 API。
9. 停机迁移并完成小范围数据验证。

实现以已合并的 `main` 为基线，在独立 `codex/` 开发分支完成；不得把用户现有的 `.claude/skills/stock` 删除项混入本功能提交。

## 19. 验收标准

1. 生产股票行情不再请求 `push2his`，新浪任意两个股票行情请求的开始时间至少间隔 5 秒。
2. 股票上游只抓日线和 qfq 因子；周线由同版本日线确定性生成。
3. 含除权历史的样例股票 Raw 与 qfq 价格符合新浪因子，既有前复权策略无需改版本。
4. 首批 8 个商品品种能够保存真实合约日线及期货专有字段。
5. sidecar 对意外空集和坏上游内容返回错误，不发布空成功数据。
6. 每根期货 Bar 可追溯至交易所、原始代码、AKShare 版本和数据版本。
7. 主力映射从决策后的下一交易日生效，不读取未来 OI 或价格。
8. `RAW_MAIN` 和 `FORWARD_RATIO` 可查询；连续序列通过前缀一致性测试。
9. 任一数据库事务失败不产生半发布版本，旧 `COMPLETE` 版本仍可读取。
10. 停机迁移后现有股票 Instrument 和版本化行情校验通过。
11. 项目总体覆盖率大于 80%，行情核心计算覆盖率大于 90%。

## 20. 外部依据

- AKShare 期货日线接口：<https://akshare.akfamily.xyz/data/futures/futures.html>
- 固定 AKShare 版本：<https://pypi.org/project/akshare/>
- AKShare 新浪 A 股实现与 qfq 因子协议：<https://github.com/akfamily/akshare/blob/main/akshare/stock/stock_zh_a_sina.py>
- 东方财富连接断开案例：<https://github.com/akfamily/akshare/issues/6592>
- 东方财富按域名替换恢复案例：<https://github.com/akfamily/akshare/issues/7230>

公开接口可能变化。实现必须通过固定 fixture 和契约测试发现变化，不能把“请求成功”直接等同于“数据正确”。
