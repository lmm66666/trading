# 商品期货日线接入设计（保留现有新浪 A 股）

## 1. 背景

当前系统通过新浪财经逐股票拉取 A 股日线和周线。新浪接口存在频率限制，全市场同步需要数小时，但在没有满足长期免费、稳定且可验证条件的替代源之前，系统接受该限制并继续使用现有链路。

系统下一阶段新增国内商品期货日线，第一批覆盖黄金、白银、原油、燃料油、焦炭、焦煤和动力煤，用于指标和信号研究。A 股数据源、同步调度和存储不在本次改造范围内。

本设计采用固定版本的 AKShare Python sidecar 采集国内期货交易所日线。Go 服务拥有领域校验、持久化、数据版本、调度、主力映射和连续合约构造的最终控制权。

### 1.1 与现有内核设计的关系

本文是期货行情接入、期货行情版本和连续合约的权威设计。它细化并覆盖《Go 策略与回测内核整体迁移设计》中以下旧假设：

- 期货数据来源不能只挂在 `Instrument` 上；同一合约的不同日期和修订可能来自不同 observation，来源必须属于 Bar 版本和 observation。
- A 股继续使用现有新浪实现。原设计中拟议的东方财富 Raw/QFQ provider 和已提交文档中拟议的 Tushare provider 均不实施。
- 期货连续合约不是外部提供方 Instrument，而是由真实合约日线、版本化主力映射和连续规则派生。

现有内核设计中的无未来函数、dataset version、指标前缀一致性和回测可复现要求继续有效。后续实施计划必须据本文同步修订相关任务，不能同时实现两套冲突的数据语义。

## 2. 目标

1. 使用隔离、固定版本、严格协议的 AKShare sidecar 接入国内期货日线，并消除“空 DataFrame 被当作成功”的风险。
2. 建立支持期货真实合约和期货连续合约的市场数据边界，同时保留结算价、持仓量等专有字段。
3. 主力换月和连续价格序列无未来数据污染、可解释、可重放，并满足前缀一致性。
4. 每一条发布的期货数据都可追溯至采集运行、原始来源、提供方版本和数据版本。
5. 新增期货能力不改变现有新浪 A 股采集行为、接口和表结构。

## 3. 非目标

- 不采集分钟线、Tick、盘口或实时行情。
- 不在第一阶段提供境外期货、外盘现货或贵金属国际报价。
- 不建设期货下单、保证金、夜盘逐笔撮合、交割和组合级回测。
- 不替换、重构或加速现有新浪 A 股日线和周线采集。
- 不引入 Tushare、东方财富或其他新的 A 股行情提供方。
- 不允许 Go 服务动态调用任意 AKShare 函数，也不接受客户端传入 URL、文件路径或 Python 代码。

## 4. 总体架构

```text
新浪财经 -> 现有 Broker/Service/Repo -> 现有 A 股日线与周线（保持不变）

交易所公开数据 <- AKShare <- Python sidecar <- Go 期货采集适配器
                                                    |
                                                    v
                                           观测层 + 版本发布
                                                    |
                                 +------------------+------------------+
                                 |                  |                  |
                           期货合约日线        数据质量事件       采集运行审计
                                 |
                           主力映射/连续序列
                                 |
                                 v
                         指标、扫描和查询数据集
```

核心原则如下：

- 以下原则只约束新增期货链路，不反向改造现有股票链路。
- AKShare 返回的是 observation，不是立即可用的业务事实。
- 数据先经过结构、单位、OHLC、日期和覆盖率校验，再作为新 dataset version 原子发布。
- Go 是唯一的业务控制面；sidecar 只是无状态的、受限的数据提取适配器。
- 原始数据不可静默覆盖。提供方修订通过版本可见区间保留历史状态。
- 真实合约价格、主力映射和连续价格分别建模，不把加工结果冒充原始行情。

## 5. 模块边界

建议新增以下模块，现有 `pkg/broker/sina.go`、`business/stock_service.go`、`business/stock_scheduler.go` 和股票仓储保持原样：

```text
internal/
  market/                         扩展 Instrument 以支持期货，不改变股票构造器
  futuresdata/
    application/                  期货同步用例、发布编排
    domain/                       期货观测、分区状态和质量规则
    provider/akshare/             Go sidecar HTTP 客户端
    futures/                      合约规范化、主力映射、连续序列
    infrastructure/mysql/         采集和行情仓储
cmd/futures-sync/                 显式日期范围的运维 CLI
sidecar/akshare-futures/          Python 服务、协议模型、适配器和测试
```

`internal/market` 保持纯领域模型，不依赖 HTTP、GORM 或具体提供方。`futuresdata/application` 只依赖 provider 和 repository port；具体网络客户端与 MySQL 实现在外层。

## 6. A 股保持现状

本次不修改 A 股数据路径：

- 历史日线继续调用新浪 `CN_MarketData.getKLineData`，`scale=240`。
- 历史周线继续调用同一接口，`scale=1680`。
- 增量日线每只股票回看 30 根，增量周线每只股票回看 10 根。
- 所有新浪请求继续共用 3 秒、burst 1 的全局限流器。
- 股票数据继续写入现有 `t_stock_kline_daily` 和 `t_stock_kline_weekly`，现有 API 行为不变。

全市场同步耗时较长是当前已接受约束。新增期货链路必须使用独立 provider、限流器、调度器和表，不得挤占新浪请求配额，也不得借本次需求改变股票同步语义。

## 7. 期货数据方案

### 7.1 首批范围

| 交易所 | 品种 | 含义 |
|---|---|---|
| SHFE | AU、AG、FU | 黄金、白银、燃料油 |
| INE | SC、LU | 原油、低硫燃料油 |
| DCE | J、JM | 焦炭、焦煤 |
| CZCE | ZC | 动力煤 |

首期仅接入国内交易所日线。品种和交易所必须来自配置白名单，未登记品种直接拒绝。

### 7.2 为什么使用 Python sidecar

AKShare 已封装 SHFE、INE、DCE、CZCE 等交易所公开数据的格式差异，适合快速覆盖多个交易所，但其上游页面和解析逻辑会变化。将这些适配逻辑直接移植到 Go 会复制大量易变代码，并长期承担与 AKShare 相同的维护成本。

因此采用薄 sidecar：Python 负责调用固定版本 AKShare 并把结果转换为稳定协议；Go 负责重试、语义校验、持久化、版本发布和所有业务派生。未来若某交易所接口足够稳定，可用新的 Go provider 替换该交易所，而不改变领域层和存储契约。

### 7.3 Sidecar 请求协议

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

### 7.4 Sidecar 错误契约

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

### 7.5 Sidecar 运行时和安全

- Python 3.12 固定 patch 版本或固定镜像 digest。
- `akshare==1.18.94`，依赖通过 lockfile 和 hash 固定；禁止启动时自动升级。
- 非 root、只读根文件系统、临时目录限额、CPU/内存/超时限制。
- 只在内部网络监听，不暴露公网。
- 出站网络仅允许配置的交易所域名。
- 不持有数据库凭据或应用密钥。
- `/livez` 只检查进程；`/readyz` 检查本地依赖和配置，不调用上游。

### 7.6 期货调度和回看

默认 18:30 首次同步，20:30 和下一交易日 08:30 重试失败分区。采集原子单位为“交易所 × 交易日”，同一分区在一个事务中发布。

每日重抓最近 10 个已观测交易日以吸收交易所修订；每月对最近 90 天做 checksum 审计。相同 payload hash 不创建无意义新版本，不同 hash 触发字段级比较和新版本发布。

夜盘数据归属遵循交易所公布的 `trade_date`，不能按请求发起时的自然日期重新解释。

## 8. 期货合约规范化

### 8.1 标识

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

### 8.2 专有字段

通用 `market.Bar` 保留 OHLCV 和时序字段。期货专有真值存入 `FuturesContractDaily`：

- `pre_settlement`、`settlement`；
- `volume_lots`、`open_interest_lots`；
- `turnover_10k_cny`；
- `statistics_basis`；
- `observation_status`；
- 来源、采集运行、提供方版本和 payload hash。

空 OHLC 必须是缺失值，不能写成零。交易所返回的 0、88、888、99 等指数或连续代码不得进入真实合约表。

交易所成交量或持仓量统计口径存在历史上的双边/单边切换，`statistics_basis` 必须随数据保存。系统不能静默把边界前后的数值当作完全同口径。FU 在 2018 年前后的合约制度差异也作为 regime boundary 保存，默认不自动跨边界构造连续序列。

### 8.3 合约元数据

`delivery_month` 从规范化合约代码确定；`listed_from`、`last_trade_date`、合约乘数、最小变动价位和 regime boundary 属于版本化合约元数据，不从每日 OHLC 行临时猜测。

元数据来源按优先级为交易所公开合约资料、受控人工导入、保守规则推断。每条元数据保存 source、effective range 和版本。无法可靠获得 `last_trade_date` 时保持为空，并使用“进入交割月”的保守强制换月规则；不得伪造精确日期。

## 9. 主力合约换月

### 9.1 策略定义

默认规则版本为 `main_oi_hysteresis_v1`。交易日 `d` 收盘后仅使用 `<= d` 的数据做出决策，映射从下一交易日生效。任何策略在 `d` 当天都不能使用当天收盘后才确定的新主力合约。

候选合约必须满足：

- 同交易所、同品种、真实合约；
- 交割月份不早于当前月份；
- 已知下一交易日可交易；
- 当日 settlement 或 close 有效；
- `volume_lots > 0`。

“下一交易日可交易”在实现上表示：合约按已知元数据在下一有效 session 仍处于挂牌期，且没有已知的终止状态。若系统尚不知道下一 session 的自然日期，则先保存决策；下一次观测到有效 session 时，才把上一 session 的决策物化为当日映射。物化过程不读取当日 OI、成交量或收盘价，因此不构成未来数据污染。

排序依次为：持仓量降序、成交量降序、交割月份升序、标准合约代码字典序。

### 9.2 常规换月和快速换月

若当前主力为 `C`，排名第一候选为 `N`：

- 常规换月：`OI(N) >= OI(C) * 1.10` 连续 2 个交易日。
- 快速换月：`OI(N) >= OI(C) * 1.25` 连续 1 个交易日。
- 最短持有期：当前主力至少保持 3 个交易日；快速换月不能绕过最短持有期。
- 防倒退：不得从较远月份回滚到更近月份。

首次出现某品种时，当日只产生选择决策，下一交易日才开始映射，避免首日策略使用收盘后信息。

### 9.3 强制换月

以下情况允许绕过确认天数和最短持有期，但仍在下一交易日生效：

- 距可靠的最后交易日不超过 5 个交易日；
- 缺少到期元数据时，当前合约已进入交割月份；
- 当前合约停止出现或明确不可交易；
- 当前合约连续 2 日无有效成交，同时远月候选有效；
- 当前持仓量为零，同时远月候选有效。

无候选时输出 `NO_ELIGIBLE_MAIN`；有候选但全部缺乏有效流动性时输出 `NO_LIQUID_MAIN`。禁止偷偷沿用已经失效的合约。

### 9.4 决策审计

每次决策保存：规则版本、决策日、生效日、旧合约、新合约、候选排序、各候选 OI/成交量/交割月、阈值、连续计数、强制原因和使用的数据版本。相同输入和规则版本必须产生相同输出。

## 10. 连续合约价格

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

## 11. 存储与版本

### 11.1 核心表

| 表 | 用途 | 关键约束 |
|---|---|---|
| `t_instruments` | 真实期货和连续合约主数据 | 业务 ID 唯一，含品种和有效期；首期不迁移股票 |
| `t_market_data_versions` | 可发布数据集版本 | 状态、范围、父版本、发布时间 |
| `t_market_bars` | 规范化通用 Bar | instrument/timeframe/open_time/版本区间 |
| `t_market_bar_observations` | 原始提供方观测 | provider/run/target/payload hash 唯一 |
| `t_market_ingestion_runs` | 期货采集运行和 checkpoint | 类型、状态、计数和错误摘要 |
| `t_market_day_states` | 市场日期观测状态 | market/date 唯一可见状态 |
| `t_trading_sessions` | 已观测或未来官方交易日历 | market/date/source/version |
| `t_futures_contract_daily` | 期货专有日线真值 | contract/date/版本区间 |
| `t_continuous_contract_mappings` | 连续合约到真实合约映射 | synthetic/date/policy/version |
| `t_futures_roll_events` | 换月决策审计 | policy/decision/effective 唯一 |
| `t_data_quality_issues` | 质量问题和处置状态 | scope/code/run/version |

版本化事实表使用 `valid_from_version`、`valid_to_version` 表示可见区间。读取版本 V 时选择 `valid_from_version <= V` 且 `valid_to_version` 为空或大于 V 的记录。

### 11.2 发布事务

每个分区遵循：

```text
FETCHED -> NORMALIZED -> VALIDATED -> STAGED -> PUBLISHED
                                  \-> QUARANTINED
```

只有 `PUBLISHED` 版本可被扫描和回测读取。发布事务同时关闭旧事实的可见区间、插入新事实、更新日状态并把版本改为 `PUBLISHED`；任一步失败则整体回滚。外部网络请求不放在数据库事务中。

## 12. 配置设计

```yaml
market_data:
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

该配置只新增期货节点，不改变现有股票配置和默认值。sidecar 地址必须是配置的内部服务地址，不能由 API 请求动态指定。

## 13. 调度、幂等与并发

- 调度器只创建带唯一业务键的任务；重复触发不会产生并行重复采集。
- 期货分区键为 `FUTURE:exchange:trade_date`。
- 同一分区同时只允许一个执行者，使用数据库租约并带过期时间，进程崩溃后可恢复。
- provider 请求带稳定 request ID；写 observation 和发布操作均为幂等 upsert。
- HTTP 超时或连接中断后，先按业务键检查 observation，再决定是否重试，避免重复发布。
- 日常同步优先于历史回补；二者使用相同并发和退避策略，避免同时压垮交易所公开接口。

## 14. API 与运维入口

当前写接口没有完善鉴权，因此第一阶段不新增可从公网直接触发期货同步的 HTTP API。写入入口仅采用内部调度器和 `futures-sync` 运维 CLI。

建议只增加只读接口：

- `GET /api/v1/futures/bars`：按真实或连续 Instrument、价格视图和 dataset version 查询日线。
- `GET /api/v1/futures/main-mappings`：查询主力映射和换月依据。
- `GET /api/v1/data-quality/issues`：查询隔离批次、缺口和待处理质量问题。

响应必须返回实际使用的 dataset version、price view、provider provenance 和数据质量状态。不能在客户端未请求时静默从派生视图降级到 Raw。

## 15. 失败处理与可观测性

关键指标包括：

- AKShare sidecar 的请求数、延迟、错误码和重试数。
- 每日期货分区行数、各品种合约覆盖数和隔离数量。
- sidecar unexpected empty、上游内容类型异常和 payload hash 变化。
- 新增/修订/未变化 Bar 数量，数据发布耗时。
- 主力换月次数、强制换月、无候选和连续因子失败数量。
- 最新成功交易日与当前日期的 lag。

日志使用结构化字段，只记录上游 body hash 和受控摘要，不记录未经清洗的响应正文。告警按“日常任务失败、数据覆盖异常、连续序列中断”分级。

## 16. 测试策略

### 16.1 Go 单元测试

- sidecar 响应字段缺失、日期不匹配、重复合约、精度和单位转换。
- 交易所分区覆盖率下降、未知空日期、重试和隔离状态机。
- 期货符号、郑商所年份解析、别名、指数代码过滤和 regime boundary。
- 主力常规/快速/强制换月、最短持有、防倒退、无候选。
- 连续因子、缺锚点、前缀一致性和 suffix poisoning。

parser、adapter、reconciler 和 continuous builder 目标覆盖率不低于 90%，项目整体覆盖率保持 80% 以上。

### 16.2 Sidecar 测试

- 使用冻结的各交易所响应 fixture 测试 AKShare 结果适配。
- 空 DataFrame、HTML 错页、JSON decode error、字段变化、超时和频控映射为明确错误。
- 协议 schema、hash、数值字符串、白名单和请求大小限制。
- pytest 覆盖率不低于 90%。

### 16.3 集成和在线冒烟

- 默认 CI 只使用 `httptest`、fixture 和本地 sidecar mock，不访问公网。
- MySQL 5.7 和 8.0 验证迁移、事务发布、租约和版本读取。
- 运行 `go test -race ./...` 检查并发安全。
- 夜间 opt-in smoke test 各请求一个最近日期，只验证契约和告警，不作为合并门禁。

## 17. 迁移顺序

1. 扩展 Instrument 和 Bar 边界以支持期货，建立期货版本、观测、运行与质量表；股票表和股票服务不迁移。
2. 上线固定版本 AKShare sidecar、Go 期货适配器、整批校验和 `futures-sync` 历史回补 CLI。
3. 先发布真实期货合约日线，验证各交易所字段和历史统计口径边界。
4. 补齐合约元数据、主力规则和审计表，再发布 `RAW_MAIN`。
5. 通过前缀一致性验证后发布 `FORWARD_RATIO` 给指标和扫描。
6. 观察至少两个完整同步周期后启用默认期货调度；新浪股票调度始终保持原样。

每个阶段都能独立回滚到上一已发布 dataset version。回滚不删除 observation 或修订历史。

## 18. 验收标准

1. 现有新浪股票 Broker、Service、Scheduler、表和 HTTP API 行为不变，项目中不新增 Tushare 或东方财富股票依赖。
2. Sidecar 对意外空集和坏上游内容返回明确错误，不发布空成功分区。
3. 每根期货合约日线都能追溯到 exchange、raw symbol、AKShare 版本、采集运行和 payload hash。
4. 主力决策在下一交易日生效，任何映射和连续价格不读取未来数据。
5. 主力映射与连续序列通过前缀一致性和 suffix poisoning 测试。
6. 质量隔离数据不会被查询、扫描或回测作为默认可用版本读取。
7. Go 与 Python 新增核心逻辑达到各自覆盖率目标，集成测试同时通过 MySQL 5.7 和 8.0。

## 19. 外部接口依据

- AKShare 期货日线接口：<https://akshare.akfamily.xyz/data/futures/futures.html>
- 固定使用的 AKShare 版本：<https://pypi.org/project/akshare/>

这些外部接口可能变化。实际实现必须用契约测试和固定 fixture 捕获变更，不能仅依赖文档描述。
