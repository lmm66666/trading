# 商品期货日线接入设计

## 1. 背景

项目已经完成策略与回测内核切换。股票行情不再经过旧 `business.StockDataService`、旧调度器和旧 K 线表，而是通过东方财富 `MarketSource` 进入统一行情应用服务，发布到版本化行情表。新浪目前保留给财报、汇率和宏观数据使用。

本次只增加国内商品期货日线，首批覆盖黄金、白银、原油、燃料油、焦炭、焦煤和动力煤。期货数据来自 AKShare 封装的交易所公开数据；Go 服务继续负责领域校验、版本发布、查询、主力合约选择和连续序列计算。

这是个人项目，允许停机更新。设计优先复用当前内核和数据库，不建设双写、灰度切换、影子表、跨进程采集集群或长期兼容层。

## 2. 当前架构带来的调整

此前设计基于旧股票链路，已经不再适用：

- 不再假设新浪承担股票日线；当前股票行情源是 `pkg/broker.EastmoneyMarketSource`。
- 不再新建平行的 `internal/futuresdata/application/domain/infrastructure` 栈。
- 期货领域值放入 `internal/market`，用例放入 `internal/application`，接口放入 `internal/port`，MySQL 实现放入现有 `internal/infrastructure/mysql`。
- AKShare Go 客户端属于外部数据源 adapter，放入 `pkg/broker`；Python 服务放入 `sidecar/akshare-futures`。
- 继续使用现有 `t_instruments`、`t_market_data_versions`、`t_market_bars` 和 `COMPLETE` 版本读取语义。
- 数据库升级采用一次停机迁移，不再设计在线多阶段切换流程。

## 3. 目标

1. 接入 SHFE、INE、DCE、CZCE 的指定商品期货日线。
2. 保存真实合约 OHLCV、结算价、前结算价、成交量、持仓量和成交额。
3. 生成无未来数据的主力合约映射、原始主力序列和比例连续序列。
4. 让现有指标、扫描和行情查询能够读取期货日线，同时不改变股票行情行为。
5. 每条期货数据可追溯到交易所、AKShare 版本、原始代码和行情版本。
6. 保持总体测试覆盖率大于 80%，期货领域计算覆盖率大于 90%。

## 4. 非目标

- 不修改当前东方财富股票行情链路，也不新增 Tushare 或新浪股票行情 adapter。
- 不采集分钟线、Tick、盘口或实时行情。
- 不接入境外期货、国际现货或场外报价。
- 不实现期货下单、保证金、交割、夜盘逐笔撮合和期货回测执行模型。
- 不允许 Go 动态调用任意 AKShare 函数，也不允许请求方传入 URL、Python 代码或文件路径。
- 不建设分布式采集、跨进程租约、Kafka、Redis 或独立数据平台。
- 不承诺交易所公开页面或 AKShare 具备商业 SLA；异常数据必须拒绝发布。

## 5. 目标架构

```text
股票行情（保持现状）
东方财富 -> EastmoneyMarketSource -> MarketIngestionService
        -> MarketDataRepository -> 版本化股票行情

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

股票行情保持当前实现：

- `EastmoneyMarketSource` 按证券和日期范围获取 Raw/QFQ 日线、周线及公司行动。
- `MarketIngestionService` 校验复权因子、日周一致性和增量窗口。
- `MarketDataRepository` 原子发布 `PENDING -> COMPLETE` 版本。
- 查询、扫描和回测固定读取一个大于 0 的 `COMPLETE` 数据版本。
- 新浪不参与股票行情发布，只继续服务现有财报、汇率和宏观用途。

期货改造不得恢复旧 `StockDataService`、旧股票调度器或旧 K 线表，也不得改变东方财富 adapter 的协议和限流。

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

### 14.1 日常同步

- 默认每天 18:30 执行一次。
- 单进程 scheduler 按配置交易所顺序调用 sidecar，并使用独立于股票行情的有界并发。
- 每次重抓最近 10 个已观测交易日；相同内容不创建新版本。
- 网络和可重试上游错误最多重试 3 次，使用带抖动的指数退避。
- 单个分区失败记录在本轮摘要，下一轮或人工命令重试，不引入持久化采集任务。
- 夜盘归属完全采用交易所数据中的 `trade_date`，不按请求时间重新解释。

### 14.2 历史回补

```text
futures-sync --from 2018-01-01 --to 2026-09-11
```

命令按日期正序执行，已存在且 digest 相同的合约跳过。失败时返回非零状态并打印最后成功日期；再次运行从指定日期安全重做即可，不额外建设 checkpoint 表。

### 14.3 查询

新增只读接口：

- `GET /api/v1/futures/bars`：真实合约、`RAW_MAIN` 或 `FORWARD_RATIO` 日线。
- `GET /api/v1/futures/main-mappings`：主力映射、决策日期和换月原因。

请求必须包含 Instrument、日期范围和价格视图，可选数据版本。版本为空时只解析一次 latest `COMPLETE`，随后固定该版本完成全部查询。

第一阶段不增加公网同步接口。同步只由内部 scheduler 或 CLI 触发。

## 15. 错误与日志

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
5. 启动新版本，先运行少量日期的 `futures-sync`。
6. 核对真实合约、主力映射和连续序列后启用日常调度。

不实施双写、影子表、在线回填和旧新 schema 长期兼容。失败时停止服务，恢复数据库备份并运行旧版本。

## 17. 测试

### 17.1 Go

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

1. 扩展 Instrument、数据库字段和迁移校验。
2. 实现期货领域值、source/writer port 和单元测试。
3. 实现固定版本 AKShare sidecar 及 fixture 测试。
4. 实现 Go adapter、真实合约发布和 MySQL 集成测试。
5. 实现主力映射与连续序列。
6. 实现 scheduler、CLI 和只读 API。
7. 停机迁移并完成小范围数据验证。

实施开始前必须以合并后的 `main` 为基线重新检查文件和接口；本设计不授权在当前 worktree 或当前功能分支继续写代码。

## 19. 验收标准

1. 股票行情仍由当前东方财富版本化链路提供，股票查询、扫描和回测行为不变。
2. 首批 8 个商品品种能够保存真实合约日线及期货专有字段。
3. sidecar 对意外空集和坏上游内容返回错误，不发布空成功数据。
4. 每根期货 Bar 可追溯至交易所、原始代码、AKShare 版本和数据版本。
5. 主力映射从决策后的下一交易日生效，不读取未来 OI 或价格。
6. `RAW_MAIN` 和 `FORWARD_RATIO` 可查询；连续序列通过前缀一致性测试。
7. 任一数据库事务失败不产生半发布版本，旧 `COMPLETE` 版本仍可读取。
8. 停机迁移后现有股票 Instrument 和版本化行情校验通过。
9. 项目总体覆盖率大于 80%，期货核心计算覆盖率大于 90%。

## 20. 外部依据

- AKShare 期货日线接口：<https://akshare.akfamily.xyz/data/futures/futures.html>
- 固定 AKShare 版本：<https://pypi.org/project/akshare/>

公开接口可能变化。实现必须通过固定 fixture 和契约测试发现变化，不能把“请求成功”直接等同于“数据正确”。
