# A 股与商品期货日线数据接入设计

## 1. 背景

当前系统主要通过新浪财经逐股票拉取 A 股日线和周线。新浪接口存在频率限制，全市场同步需要数小时；采集链路也把数据源、业务编排、增量判断和存储耦合在一起，难以增加新的数据源或对采集结果做可追溯审计。

系统下一阶段只需要两类行情：

- A 股日线，用于技术指标、信号扫描和股票回测。
- 国内商品期货日线，第一批覆盖黄金、白银、原油、燃料油、焦炭、焦煤和动力煤，用于指标和信号研究。

本设计采用“Tushare 主源 + 新浪小范围补缺”采集 A 股，采用固定版本的 AKShare Python sidecar 采集国内期货交易所日线。Go 服务仍然拥有领域校验、数据版本、持久化、调度、主力映射和连续合约构造的最终控制权。

### 1.1 与现有内核设计的关系

本文是行情接入、行情版本和期货连续合约的权威设计。它细化并覆盖《Go 策略与回测内核整体迁移设计》中以下旧假设：

- 数据来源不能只挂在 `Instrument` 上；同一 Instrument 的不同日期、修订和字段可能来自不同 observation，来源必须属于 Bar 版本和 observation。
- A 股默认价格不再依赖东方财富 Raw/QFQ 双接口；Raw 来自 Tushare/新浪，策略默认派生价格使用本文定义的 `ForwardAdjusted`。
- 期货连续合约不是外部提供方 Instrument，而是由真实合约日线、版本化主力映射和连续规则派生。

现有内核设计中的无未来函数、dataset version、指标前缀一致性和回测可复现要求继续有效。后续实施计划必须据本文同步修订相关任务，不能同时实现两套冲突的数据语义。

## 2. 目标

1. 将 A 股日常全市场增量同步从数小时缩短到分钟级，正常情况下每个交易日只调用一次 Tushare 日线接口。
2. 在免费额度内稳定运行，并通过应用侧双重限流避免单次异常耗尽额度。
3. 新浪只修复少量、可验证的个股缺口，禁止在主源异常时形成全市场回退风暴。
4. 股票周线由已验证的日线在本地聚合，不再独立请求外部周线。
5. 使用隔离、固定版本、严格协议的 AKShare sidecar 接入国内期货日线，并消除“空 DataFrame 被当作成功”的风险。
6. 建立支持股票、期货合约和期货连续合约的统一市场数据边界，同时保留期货结算价、持仓量等专有字段。
7. 主力换月和连续价格序列无未来数据污染、可解释、可重放，并满足前缀一致性。
8. 每一条发布数据都可追溯至采集运行、原始来源、提供方版本和数据版本。

## 3. 非目标

- 不采集分钟线、Tick、盘口或实时行情。
- 不在第一阶段提供境外期货、外盘现货或贵金属国际报价。
- 不建设期货下单、保证金、夜盘逐笔撮合、交割和组合级回测。
- 不允许 Go 服务动态调用任意 AKShare 函数，也不接受客户端传入 URL、文件路径或 Python 代码。
- 不承诺用免费 Tushare 获取交易日历、停复牌、证券主数据或官方复权因子；这些能力需要更高积分时再单独启用。
- 不把传统“以最新价锚定、历史会随未来数据变化”的前复权序列作为策略默认输入。

## 4. 总体架构

```text
                        +----------------------+
Tushare HTTPS -------->| Go 股票采集适配器   |----+
新浪 HTTP/HTTPS ------>| 小范围缺口修复适配器 |    |
                        +----------------------+    v
                                              规范化/校验
                                                    |
交易所公开数据 <- AKShare <- Python sidecar <- Go 期货采集适配器
                                                    |
                                                    v
                                           观测层 + 版本发布
                                                    |
                         +--------------------------+------------------+
                         |                          |                  |
                    股票原始日线              期货合约日线       数据质量事件
                         |                          |
                    本地周线聚合              主力映射/连续序列
                         +--------------------------+
                                                    v
                                       指标、扫描、查询、回测数据集
```

核心原则如下：

- 外部提供方返回的是 observation，不是立即可用的业务事实。
- 数据先经过结构、单位、OHLC、日期、覆盖率和来源优先级校验，再作为新 dataset version 原子发布。
- Go 是唯一的业务控制面；sidecar 只是无状态的、受限的数据提取适配器。
- 原始数据不可静默覆盖。提供方修订通过版本可见区间保留历史状态。
- 原始价格、派生价格、主力映射和连续价格分别建模，不把不可逆加工结果冒充原始行情。

## 5. 模块边界

建议新增以下模块：

```text
internal/
  market/                         通用 Instrument、Bar、Dataset、版本值对象
  marketdata/
    application/                  股票/期货同步用例、发布编排、补缺策略
    domain/                       观测、日状态、质量规则、来源优先级
    provider/tushare/             原生 Go Tushare 客户端
    provider/sina/                现有新浪适配器的受限补缺封装
    provider/akshare/             Go sidecar HTTP 客户端
    futures/                      合约规范化、主力映射、连续序列
    aggregate/                    日线到周线聚合
    infrastructure/mysql/         采集和行情仓储
cmd/market-sync/                  显式日期范围的运维 CLI
sidecar/akshare-futures/          Python 服务、协议模型、适配器和测试
```

`internal/market` 保持纯领域模型，不依赖 HTTP、GORM 或具体提供方。`marketdata/application` 只依赖 provider 和 repository port；具体网络客户端与 MySQL 实现在外层。

## 6. A 股日线方案

### 6.1 Tushare 原生 Go 接入

Tushare 使用简单的 HTTPS JSON POST 协议，无需引入第三方 Go SDK。请求体固定为：

```json
{
  "api_name": "daily",
  "token": "${TUSHARE_TOKEN}",
  "params": {"trade_date": "20260911"},
  "fields": "ts_code,trade_date,open,high,low,close,pre_close,change,pct_chg,vol,amount"
}
```

客户端职责：

- 只允许调用配置中登记的 API 名称，本阶段生产路径只开放 `daily`。
- Token 仅从环境变量读取，禁止进入 YAML、日志、错误消息或 tracing attribute。
- 强制 HTTPS、固定 host、连接和响应超时、响应体大小上限。
- 使用 `json.RawMessage` 或 `Decoder.UseNumber`，再通过十进制定点解析，禁止先转 `float64` 后做金额或数量单位换算。
- 逐项解析 `data.fields` 和 `data.items`，不能依赖列的隐式固定顺序。
- Tushare `code != 0`、字段缺失、行宽不一致、日期不匹配或重复证券都视为整批失败，不发布部分结果。

官方 `daily` 接口按 `trade_date` 查询可一次返回全市场，单次上限为 6000 行。因此正常日增量按交易日请求，而不是逐股票请求。

### 6.2 单位和精度

规范化后使用确定性定点值：

| Tushare 字段 | 提供方单位 | 领域单位 | 转换 |
|---|---:|---:|---:|
| `open/high/low/close/pre_close` | 元 | 元，精度 1e-4 | 十进制解析 |
| `vol` | 手 | 股 | 乘 100 |
| `amount` | 千元 | 元 | 乘 1000 |

`change` 和 `pct_chg` 只作为校验辅助值，不作为核心 Bar 真值。OHLC 必须满足 `high >= max(open, close, low)`、`low <= min(open, close, high)`，价格与成交量不得为负。

### 6.3 限流与额度

以免费 120 积分的保守权限为基线：

- Token bucket：每分钟 45 次，`burst=1`。
- 应用日预算：7500 次，低于官方 8000 次上限，给人工排障和时钟误差留余量。
- 所有重试也计入日预算。
- 429、提供方频控消息和网络暂时错误采用带随机抖动的指数退避；参数或权限错误不重试。
- 预算不足以完成显式历史回补时保存 checkpoint 并暂停，下一额度周期继续。

额度由持久化计数器按“provider + token fingerprint + 自然日”统计。fingerprint 只保存不可逆摘要，不保存 Token。

### 6.4 日常同步

默认每天 17:15 执行：

1. 确定最近 5 个“已完成候选日”，逐日调用 `daily(trade_date)`；该回看窗口用于吸收迟到和修订。
2. 每个日期先写 observation，完成整批校验和覆盖率判断。
3. 合格批次与当前可见版本比较，只为新增或变化行创建新版本记录。
4. 同一日期的数据、日状态和 dataset version 在一个事务中发布。
5. 发布完成后聚合受影响周的股票周线。

一次返回恰好 6000 行时视为疑似截断，整批隔离并产生 `SUSPECTED_TRUNCATION`，不得发布。当前批次行数比最近 20 个已完成交易日中位数下降超过 5% 时产生软质量告警；补缺结束前该版本不进入 `COMPLETE`。

### 6.5 免费权限下的交易日推断

免费层不能依赖 `trade_cal`、`stock_basic` 和 `suspend_d`。系统用全市场批次观测维护日状态：

- 当前或未来附近日期返回空集：`EMPTY_PENDING`，先重试，不能立即断言休市。
- 历史空日期在前后存在正常完整批次，且经过配置次数重试后：`NO_SESSION_INFERRED`。
- 非空并通过整批校验：`SESSION_OBSERVED`。
- 非空但结构或覆盖异常：`QUARANTINED`。

不得为了制造连续日历而向原始 Bar 表插入伪造 K 线。

### 6.6 股票池快照

补缺比例和覆盖率必须相对于版本化的 `equity universe snapshot` 计算，不能直接把“当天返回的股票”既当分子又当分母。首个快照由现有股票主数据与首次成功全市场批次的并集生成；之后按以下规则演进：

- 新代码在完整批次中首次出现时加入，记录 `first_seen_date`。
- 已知代码连续缺失不能自动解释为退市，先保留为 active 并记录日状态。
- 只有人工导入的交易所清单、后续启用的可信证券主数据接口，或明确的退市证据才能设置 `inactive_from`。
- 每次同步把使用的 universe version 写入运行和数据版本，保证覆盖率判断可重放。

初次启动尚无可信快照时允许发布 Tushare 完整批次，但状态为 `BASELINE_BUILDING`，不触发新浪补缺。积累至少 5 个正常批次后才启用相对覆盖率和缺口修复。

### 6.7 新浪补缺边界

新浪仅在 Tushare 某日全市场批次成功且覆盖看起来正常后补个股缺口。单日同时满足以下约束才启动：

- 待修复数量不超过 100 只；
- 待修复数量不超过预期股票池的 1%；
- 新浪返回的数据日期必须精确等于目标日期；
- OHLC、成交量和证券代码通过同一套领域校验。

Tushare 整批失败、空集、疑似截断或覆盖异常时，不允许向新浪逐股 fan-out。此时保持旧版本可见，并记录批次失败。

来源优先级为 `Tushare=20`、`Sina=10`、`Legacy=5`。高优先级可替代低优先级，低优先级不能覆盖高优先级。来源权威性以整行 Bar 为单位，禁止把一个来源的价格和另一个来源的成交量拼成一行。

### 6.8 个股无数据状态

在已知交易日中缺少某只股票时记录 `NO_BAR_UNKNOWN`。若前后交易日都有该证券且缺口稳定，可额外标记 `SUSPENDED_INFERRED`，但它仍是推断状态。

数据集加载器可在内存中为停牌日构造“不可交易时钟点”：价格沿用上一有效收盘、成交量为 0、`TradingStatus=Suspended`。该时钟点仅服务多标的日历对齐，不能写入提供方原始 Bar 表，也不能参与成交。

### 6.9 历史回补

历史回补只通过显式 CLI 启动，例如：

```text
market-sync equity --from 2018-01-01 --to 2026-09-11
```

CLI 按日期正序执行、每个日期形成独立 checkpoint，支持幂等恢复。达到日预算时正常退出为 `PAUSED_BUDGET`，而不是把未完成任务标为成功或无限重试。

## 7. 股票复权与周线

### 7.1 两种价格视图

数据库持久化提供方原始日线 `Raw`。在没有官方复权因子的免费方案中，额外提供无未来依赖的 `ForwardAdjusted` 派生视图：

```text
factor(first) = 1
factor(d) = factor(prev) * rawClose(prev) / preClose(d)
linkedOHLC(d) = rawOHLC(d) * factor(d)
```

其中 `prev` 是该股票上一根真实、有效的交易 Bar，不是上一自然日。成交量保持原值，不随价格因子调整。

这个序列的早期前缀稳定，适合策略和回测重放，但它不是行情软件常见的“以最新价格锚定”的传统前复权。API 和文档必须使用明确名称，不能简称为 `qfq`。若 `pre_close` 缺失、为零或与前一有效收盘无法形成合理比值，Raw 仍可发布，派生视图则在该处产生质量错误并停止延伸。

### 7.2 周线聚合

股票周线完全由已发布日线派生：

- Open：该交易周第一根有效日线开盘价。
- High：周内最高价。
- Low：周内最低价。
- Close：周内最后一根有效日线收盘价。
- Volume、Amount：周内求和。
- CloseTime：该周实际最后一个已观测交易日的收盘时间。

Raw 和 ForwardAdjusted 分别聚合，禁止先聚合 Raw 周线再用单一周因子粗略复权。任一日线版本变化时，重新计算对应自然周并发布新的周线版本。

## 8. 期货数据方案

### 8.1 首批范围

| 交易所 | 品种 | 含义 |
|---|---|---|
| SHFE | AU、AG、FU | 黄金、白银、燃料油 |
| INE | SC、LU | 原油、低硫燃料油 |
| DCE | J、JM | 焦炭、焦煤 |
| CZCE | ZC | 动力煤 |

首期仅接入国内交易所日线。品种和交易所必须来自配置白名单，未登记品种直接拒绝。

### 8.2 为什么使用 Python sidecar

AKShare 已封装 SHFE、INE、DCE、CZCE 等交易所公开数据的格式差异，适合快速覆盖多个交易所，但其上游页面和解析逻辑会变化。将这些适配逻辑直接移植到 Go 会复制大量易变代码，并长期承担与 AKShare 相同的维护成本。

因此采用薄 sidecar：Python 负责调用固定版本 AKShare 并把结果转换为稳定协议；Go 负责重试、语义校验、持久化、版本发布和所有业务派生。未来若某交易所接口足够稳定，可用新的 Go provider 替换该交易所，而不改变领域层和存储契约。

### 8.3 Sidecar 请求协议

唯一业务端点：

```http
POST /v1/futures/daily-partition
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

一次请求只允许一个交易所、一个交易日和白名单内的有限品种集合。成功响应示例：

```json
{
  "schema_version": 1,
  "provider": "akshare",
  "provider_version": "1.18.94",
  "exchange": "SHFE",
  "trade_date": "2026-09-11",
  "status": "OK",
  "coverage": {"AU": 8, "AG": 10, "FU": 6},
  "row_count": 24,
  "payload_sha256": "...",
  "fetched_at": "2026-09-11T18:31:12+08:00",
  "source_url": "https://...",
  "rows": []
}
```

每行至少包含 `raw_symbol`、标准化 `product`、`trade_date`、OHLC、`settlement`、`pre_settlement`、`volume_lots`、`open_interest_lots`、`turnover_10k_cny` 和 `statistics_basis`。数值以 JSON 字符串传输，由 Go 十进制解析。

仅当 sidecar 能验证该日休市或品种尚未上市时，才允许 `200` 返回空结果并给出明确状态。无法解释的空 DataFrame 必须转为错误，不能视为成功。

### 8.4 Sidecar 错误契约

| HTTP | 错误码 | 是否可重试 |
|---:|---|---|
| 400 | `INVALID_ARGUMENT` | 否 |
| 422 | `UNSUPPORTED_EXCHANGE_OR_DATE` | 否 |
| 502 | `UPSTREAM_BAD_PAYLOAD` | 视原因 |
| 502 | `UPSTREAM_UNEXPECTED_EMPTY` | 是 |
| 503 | `UPSTREAM_UNAVAILABLE` | 是 |
| 503 | `UPSTREAM_RATE_LIMITED` | 是 |
| 504 | `UPSTREAM_TIMEOUT` | 是 |

错误响应包含 `request_id`、`retryable`、`upstream_status`、`content_type` 和 `body_hash`。禁止把上游 HTML、Cookie、完整 URL 查询参数或堆栈直接返回给 Go 或写入日志。

### 8.5 Sidecar 运行时和安全

- Python 3.12 固定 patch 版本或固定镜像 digest。
- `akshare==1.18.94`，依赖通过 lockfile 和 hash 固定；禁止启动时自动升级。
- 非 root、只读根文件系统、临时目录限额、CPU/内存/超时限制。
- 只在内部网络监听，不暴露公网。
- 出站网络仅允许配置的交易所域名。
- 不持有数据库凭据、Tushare Token 或应用密钥。
- `/livez` 只检查进程；`/readyz` 检查本地依赖和配置，不调用上游。

### 8.6 期货调度和回看

默认 18:30 首次同步，20:30 和下一交易日 08:30 重试失败分区。采集原子单位为“交易所 × 交易日”，同一分区在一个事务中发布。

每日重抓最近 10 个已观测交易日以吸收交易所修订；每月对最近 90 天做 checksum 审计。相同 payload hash 不创建无意义新版本，不同 hash 触发字段级比较和新版本发布。

夜盘数据归属遵循交易所公布的 `trade_date`，不能按请求发起时的自然日期重新解释。

## 9. 期货合约规范化

### 9.1 标识

真实合约使用完整四位年份月份：

```text
SHFE:AU202612
INE:SC202612
CZCE:ZC202609
```

连续合约是独立的合成 Instrument：

```text
SHFE:AU.MAIN
INE:SC.MAIN
```

原始代码必须保存在 `raw_symbol`。对历史别名如 `TC -> ZC` 使用带有效期的映射规则，规范化后仍保留来源符号和规则版本，避免历史语义丢失。

`Instrument` 扩展为：

- `AssetClass`: `EQUITY`、`FUTURE`。
- `InstrumentKind`: `SPOT_EQUITY`、`FUTURE_CONTRACT`、`FUTURE_CONTINUOUS`。
- `Exchange`: 在 SSE、SZSE、BSE 基础上增加 SHFE、INE、DCE、CZCE、GFEX。

现有“仅六位数字股票代码”的校验必须移到股票专用构造器，不能继续作为通用 Instrument 约束。

### 9.2 专有字段

通用 `market.Bar` 保留 OHLCV 和时序字段。期货专有真值存入 `FuturesContractDaily`：

- `pre_settlement`、`settlement`；
- `volume_lots`、`open_interest_lots`；
- `turnover_10k_cny`；
- `statistics_basis`；
- `observation_status`；
- 来源、采集运行、提供方版本和 payload hash。

空 OHLC 必须是缺失值，不能写成零。交易所返回的 0、88、888、99 等指数或连续代码不得进入真实合约表。

交易所成交量或持仓量统计口径存在历史上的双边/单边切换，`statistics_basis` 必须随数据保存。系统不能静默把边界前后的数值当作完全同口径。FU 在 2018 年前后的合约制度差异也作为 regime boundary 保存，默认不自动跨边界构造连续序列。

### 9.3 合约元数据

`delivery_month` 从规范化合约代码确定；`listed_from`、`last_trade_date`、合约乘数、最小变动价位和 regime boundary 属于版本化合约元数据，不从每日 OHLC 行临时猜测。

元数据来源按优先级为交易所公开合约资料、受控人工导入、保守规则推断。每条元数据保存 source、effective range 和版本。无法可靠获得 `last_trade_date` 时保持为空，并使用“进入交割月”的保守强制换月规则；不得伪造精确日期。

## 10. 主力合约换月

### 10.1 策略定义

默认规则版本为 `main_oi_hysteresis_v1`。交易日 `d` 收盘后仅使用 `<= d` 的数据做出决策，映射从下一交易日生效。任何策略在 `d` 当天都不能使用当天收盘后才确定的新主力合约。

候选合约必须满足：

- 同交易所、同品种、真实合约；
- 交割月份不早于当前月份；
- 已知下一交易日可交易；
- 当日 settlement 或 close 有效；
- `volume_lots > 0`。

“下一交易日可交易”在实现上表示：合约按已知元数据在下一有效 session 仍处于挂牌期，且没有已知的终止状态。若系统尚不知道下一 session 的自然日期，则先保存决策；下一次观测到有效 session 时，才把上一 session 的决策物化为当日映射。物化过程不读取当日 OI、成交量或收盘价，因此不构成未来数据污染。

排序依次为：持仓量降序、成交量降序、交割月份升序、标准合约代码字典序。

### 10.2 常规换月和快速换月

若当前主力为 `C`，排名第一候选为 `N`：

- 常规换月：`OI(N) >= OI(C) * 1.10` 连续 2 个交易日。
- 快速换月：`OI(N) >= OI(C) * 1.25` 连续 1 个交易日。
- 最短持有期：当前主力至少保持 3 个交易日；快速换月不能绕过最短持有期。
- 防倒退：不得从较远月份回滚到更近月份。

首次出现某品种时，当日只产生选择决策，下一交易日才开始映射，避免首日策略使用收盘后信息。

### 10.3 强制换月

以下情况允许绕过确认天数和最短持有期，但仍在下一交易日生效：

- 距可靠的最后交易日不超过 5 个交易日；
- 缺少到期元数据时，当前合约已进入交割月份；
- 当前合约停止出现或明确不可交易；
- 当前合约连续 2 日无有效成交，同时远月候选有效；
- 当前持仓量为零，同时远月候选有效。

无候选时输出 `NO_ELIGIBLE_MAIN`；有候选但全部缺乏有效流动性时输出 `NO_LIQUID_MAIN`。禁止偷偷沿用已经失效的合约。

### 10.4 决策审计

每次决策保存：规则版本、决策日、生效日、旧合约、新合约、候选排序、各候选 OI/成交量/交割月、阈值、连续计数、强制原因和使用的数据版本。相同输入和规则版本必须产生相同输出。

## 11. 连续合约价格

系统提供两个明确视图：

1. `RAW_MAIN`：按主力映射拼接真实合约原始价格，换月处允许跳空。用于来源审计和未来真实合约执行研究。
2. `FORWARD_RATIO`：默认指标视图，保持最早历史段不变，在换月后缩放新合约及其未来后缀。

换月决策日的旧、新合约都有有效 close 时：

```text
q(d) = close(new, d) / close(old, d)
factor(new segment) = factor(old segment) / q(d)
adjustedPrice = rawPrice * segmentFactor
```

同一因子作用于 OHLC 和 settlement；成交量、持仓量不调整。v1 固定以 Close 为换月锚点，规则升级必须变更版本号。

若锚点任一价格缺失，主力映射仍可按规则换月，`RAW_MAIN` 继续发布；`FORWARD_RATIO` 从该点停止并记录 `NO_ROLL_FACTOR`，不能用未来某天价格回填当前因子。

传统“当前端锚定”的后复权/前复权连续序列会在新增未来数据后重写全部历史，只能作为带 `as_of` 的查询时派生视图，不能成为策略默认输入。

所有派生器必须满足：

```text
Build(all)[:k] == Build(all[:k])
```

测试还要对截点后的输入做 suffix poisoning，证明未来价格、未来 OI 和未来合约不会改变截点前的主力映射或连续价格。

## 12. 存储与版本

### 12.1 核心表

| 表 | 用途 | 关键约束 |
|---|---|---|
| `t_instruments` | 股票、真实期货和连续合约主数据 | 业务 ID 唯一，含品种和有效期 |
| `t_market_data_versions` | 可发布数据集版本 | 状态、范围、父版本、发布时间 |
| `t_market_bars` | 规范化通用 Bar | instrument/timeframe/open_time/版本区间 |
| `t_market_bar_observations` | 原始提供方观测 | provider/run/target/payload hash 唯一 |
| `t_market_ingestion_runs` | 采集运行和 checkpoint | 类型、状态、预算、计数和错误摘要 |
| `t_market_day_states` | 市场日期观测状态 | market/date 唯一可见状态 |
| `t_trading_sessions` | 已观测或未来官方交易日历 | market/date/source/version |
| `t_instrument_daily_states` | 个股停牌/未知缺口状态 | instrument/date/version |
| `t_futures_contract_daily` | 期货专有日线真值 | contract/date/版本区间 |
| `t_continuous_contract_mappings` | 连续合约到真实合约映射 | synthetic/date/policy/version |
| `t_futures_roll_events` | 换月决策审计 | policy/decision/effective 唯一 |
| `t_data_quality_issues` | 质量问题和处置状态 | scope/code/run/version |

版本化事实表使用 `valid_from_version`、`valid_to_version` 表示可见区间。读取版本 V 时选择 `valid_from_version <= V` 且 `valid_to_version` 为空或大于 V 的记录。

### 12.2 发布事务

每个分区遵循：

```text
FETCHED -> NORMALIZED -> VALIDATED -> STAGED -> PUBLISHED
                                  \-> QUARANTINED
```

只有 `PUBLISHED` 版本可被扫描和回测读取。发布事务同时关闭旧事实的可见区间、插入新事实、更新日状态并把版本改为 `PUBLISHED`；任一步失败则整体回滚。外部网络请求不放在数据库事务中。

## 13. 配置设计

```yaml
market_data:
  equity_daily:
    enabled: true
    schedule: "17:15"
    lookback_sessions: 5
    primary: tushare
    tushare:
      base_url: "https://api.tushare.pro"
      token_env: "TUSHARE_TOKEN"
      rate_per_minute: 45
      burst: 1
      daily_budget: 7500
    repair:
      provider: sina
      max_symbols: 100
      max_ratio: 0.01

  futures_daily:
    enabled: true
    schedule: "18:30"
    retry_schedules: ["20:30", "next-day 08:30"]
    recent_recheck_sessions: 10
    sidecar_url: "http://akshare-futures:8081"
    exchanges:
      SHFE: [AU, AG, FU]
      INE: [SC, LU]
      DCE: [J, JM]
      CZCE: [ZC]
    continuous:
      main_policy: "main_oi_hysteresis_v1"
      price_view: "forward_ratio_v1"
```

生产 Docker 镜像不再复制 `config-nas.yaml`。配置通过运行时只读挂载提供，Token 通过 secret 或环境变量注入。

## 14. 调度、幂等与并发

- 调度器只创建带唯一业务键的任务；重复触发不会产生并行重复采集。
- 股票分区键为 `EQUITY:trade_date`，期货分区键为 `FUTURE:exchange:trade_date`。
- 同一分区同时只允许一个执行者，使用数据库租约并带过期时间，进程崩溃后可恢复。
- provider 请求带稳定 request ID；写 observation 和发布操作均为幂等 upsert。
- HTTP 超时或连接中断后，先按业务键检查 observation，再决定是否重试，避免重复发布。
- 正常调度与历史回补共享额度计数器；日常任务优先于回补任务。

## 15. API 与运维入口

当前写接口没有完善鉴权，因此第一阶段不新增可从公网直接触发全市场同步的 HTTP API。写入入口采用内部调度器和运维 CLI；现有兼容写 API 最多只能入队受控的小批次任务。

建议只增加只读接口：

- `GET /api/v1/market/bars`：按 Instrument、周期、价格视图和 dataset version 查询。
- `GET /api/v1/futures/main-mappings`：查询主力映射和换月依据。
- `GET /api/v1/data-quality/issues`：查询隔离批次、缺口和待处理质量问题。

响应必须返回实际使用的 dataset version、price view、provider provenance 和数据质量状态。不能在客户端未请求时静默从派生视图降级到 Raw。

## 16. 失败处理与可观测性

关键指标包括：

- 每个 provider 的请求数、延迟、错误码、重试数和额度余量。
- 每日分区行数、相对 20 日中位数偏差、补缺数量和隔离数量。
- sidecar unexpected empty、上游内容类型异常和 payload hash 变化。
- 新增/修订/未变化 Bar 数量，数据发布耗时。
- 主力换月次数、强制换月、无候选和连续因子失败数量。
- 最新成功交易日与当前日期的 lag。

日志使用结构化字段，只记录 Token fingerprint，不记录 Token；只记录上游 body hash 和受控摘要，不记录未经清洗的响应正文。告警按“日常任务失败、数据覆盖异常、连续序列中断、额度即将耗尽”分级。

## 17. 测试策略

### 17.1 Go 单元测试

- Tushare fields/items 任意列顺序、缺列、行宽错误、非零 code、精度和单位转换。
- 6000 行截断、覆盖率下降、空日期状态机和预算耗尽。
- 来源优先级、整行权威、Sina 补缺上限和主源失败时禁止 fan-out。
- 复权链接、缺失 pre_close、停牌跨越和周线聚合。
- 期货符号、郑商所年份解析、别名、指数代码过滤和 regime boundary。
- 主力常规/快速/强制换月、最短持有、防倒退、无候选。
- 连续因子、缺锚点、前缀一致性和 suffix poisoning。

parser、adapter、reconciler 和 continuous builder 目标覆盖率不低于 90%，项目整体覆盖率保持 80% 以上。

### 17.2 Sidecar 测试

- 使用冻结的各交易所响应 fixture 测试 AKShare 结果适配。
- 空 DataFrame、HTML 错页、JSON decode error、字段变化、超时和频控映射为明确错误。
- 协议 schema、hash、数值字符串、白名单和请求大小限制。
- pytest 覆盖率不低于 90%。

### 17.3 集成和在线冒烟

- 默认 CI 只使用 `httptest`、fixture 和本地 sidecar mock，不访问公网。
- MySQL 5.7 和 8.0 验证迁移、事务发布、租约和版本读取。
- 运行 `go test -race ./...` 检查并发安全。
- 夜间 opt-in smoke test 各请求一个最近日期，只验证契约和告警，不作为合并门禁。

## 18. 迁移顺序

1. 扩展 Instrument 和 Bar 边界，建立版本、观测、运行与质量表。
2. 实现 Tushare Go 客户端、整批校验和显式历史回补 CLI。
3. 将日常股票采集切到 Tushare，并把新浪限制为补缺；旧新浪全量路径先保留为禁用回滚开关。
4. 上线无未来依赖的 ForwardAdjusted 和本地周线派生。
5. 上线固定版本 AKShare sidecar 与 Go 期货适配器，先只发布真实合约日线。
6. 补齐合约元数据、主力规则和审计表，再发布 `RAW_MAIN`。
7. 通过前缀一致性验证后发布 `FORWARD_RATIO` 给指标和扫描。
8. 观察至少两个完整同步周期后删除运行时对旧周线外部采集的依赖；旧表删除另走备份和运维变更。

每个阶段都能独立回滚到上一已发布 dataset version。回滚不删除 observation 或修订历史。

## 19. 验收标准

1. 正常 A 股交易日只需一次 Tushare `daily(trade_date)` 请求，日常增量目标在 2 分钟内完成。
2. 单日新浪修复不超过 100 只且不超过股票池 1%；Tushare 整批异常时新浪请求数为 0。
3. 股票周线全部从已发布日线派生，任一日线修订能重建对应周线版本。
4. Token 不出现在配置文件、日志、错误响应、trace 或数据库明文中。
5. Sidecar 对意外空集和坏上游内容返回明确错误，不发布空成功分区。
6. 每根期货合约日线都能追溯到 exchange、raw symbol、AKShare 版本、采集运行和 payload hash。
7. 主力决策在下一交易日生效，任何映射和连续价格不读取未来数据。
8. 主力映射与连续序列通过前缀一致性和 suffix poisoning 测试。
9. 质量隔离数据不会被查询、扫描或回测作为默认可用版本读取。
10. Go 与 Python 新增核心逻辑达到各自覆盖率目标，集成测试同时通过 MySQL 5.7 和 8.0。

## 20. 外部接口依据

- Tushare HTTP API 请求协议：<https://tushare.pro/document/1?doc_id=130>
- Tushare A 股日线接口和字段：<https://tushare.pro/document/1?doc_id=27>
- Tushare 积分及频次权限：<https://tushare.pro/document/1?doc_id=290>
- Tushare 复权因子权限：<https://tushare.pro/document/2?doc_id=28>
- AKShare 期货日线接口：<https://akshare.akfamily.xyz/data/futures/futures.html>
- 固定使用的 AKShare 版本：<https://pypi.org/project/akshare/>

这些外部接口可能变化。实际实现必须用契约测试和固定 fixture 捕获变更，不能仅依赖文档描述。
