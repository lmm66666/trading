---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: []
related: []
---

# API 接口文档

## 通用说明

所有接口均采用统一的 JSON 响应格式：

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

| 字段    | 类型   | 说明                        |
|---------|--------|-----------------------------|
| code    | int    | 0 表示成功，非 0 表示错误   |
| message | string | 提示信息                    |
| data    | any    | 业务数据，错误时为 null     |

---

## 接口列表

### V1 持久化策略任务

创建和取消返回202，状态和结果读取返回200。统一响应为 `{code,message,data}`；错误时 code 为 HTTP 状态整数，message 为以下稳定标识，data 为 null。

| HTTP | message | 条件 |
|---|---|---|
| 400 | INVALID_REQUEST | JSON、字段、身份长度、证券、日期、参数或分页非法 |
| 404 | NOT_FOUND | 策略/版本、证券/行情或对应类型的 Run 不存在 |
| 409 | IDEMPOTENCY_CONFLICT | 同种任务幂等键已绑定不同输入 |
| 409 | SIGNAL_SNAPSHOT_NOT_READY | 无匹配的已发布快照 |
| 409 | RUN_RESULT_NOT_READY | 回测结果尚未成功发布，包括失败/取消 |
| 409 | AMBIGUOUS_INSTRUMENT | 旧六位代码匹配多个活跃交易所证券 |
| 429 | MARKET_REFRESH_ALREADY_RUNNING | 已有定时或手工全市场行情刷新运行中 |
| 500 | internal server error | 未分类内部错误；不含 SQL、凭据、路径或堆栈 |

JSON 创建请求限制1MiB（含空白），拒绝未知字段、超大 body 和尾随第二个 JSON 值。时间使用 RFC3339，输入时区规范化为 UTC；日期范围最多20年。身份精确区分大小写和尾空格：策略/Run/SnapshotID 最多64字节，策略版本32字节，幂等键128字节。证券使用完整 `SSE:600000`、`SZSE:000001` 或 `BSE:920001`；期货主力连续使用 `SHFE:AU.MAIN` 这类完整身份。

#### 创建回测

`POST /api/v1/backtest-runs`：`instrument,strategy,strategy_version,idempotency_key,start,end,config` 必填，`parameters` 是可选策略参数对象，白名单和范围从策略目录读取。

```bash
curl -X POST http://localhost:8080/api/v1/backtest-runs \
  -H 'Content-Type: application/json' \
  -d '{"instrument":"SSE:600000","strategy":"daily_b1_buy","strategy_version":"1","idempotency_key":"backtest-example-1","start":"2026-01-01T00:00:00Z","end":"2026-06-01T00:00:00Z","config":{"initial_cash":10000000000,"cash_fraction_bps":10000,"commission_bps":3,"minimum_commission":50000,"stamp_duty_bps":5,"transfer_fee_bps":0,"slippage_bps":5,"lot_size":100,"hold_bars":10}}'
```

Price/Money 均为有符号整数，缩放10000：initial_cash=10000000000 表示100万元，minimum_commission=50000 表示5元。大整数客户端应使用无损 JSON/BigInt，避免先转 JavaScript Number。cash_fraction_bps 为1–10000，费率/滑点为0–10000bps，lot_size>0，hold_bars>=0；省略费用为0，hold_bars=0 使用策略默认持有期。上例仅演示执行假设，不是费率建议。

响应 `{"code":0,"message":"success","data":{"run_id":"...","status":"PENDING"}}`。同 kind、幂等键和相同请求返回原任务，即使最新行情已更新；修改配置复用同一键返回409。重复任务可能已运行或终态，创建仍返回202。

#### 回测状态、取消与结果页

| 方法 | 路径 | 数据 |
|---|---|---|
| GET | `/api/v1/backtest-runs/:run_id` | 安全状态元数据；成功时附 summary |
| POST | `/api/v1/backtest-runs/:run_id/cancel` | `{run_id,cancel_requested:true}` |
| GET | `/api/v1/backtest-runs/:run_id/orders` | 订单 Page |
| GET | `/api/v1/backtest-runs/:run_id/trades` | 成交 Fill Page，不是往返交易对 |
| GET | `/api/v1/backtest-runs/:run_id/equity` | 权益 Page |

取消请求体可为空或空对象，不接受额外字段。取消表示请求已交给持久化队列；终态任务保持终态，重新 GET 核对状态。错误 Run 类型返回404，不会取消另一种任务。

状态数据含 `run_id,status,kind,strategy,strategy_version,data_version,engine_version,attempts,cancel_requested_at`。status 取 PENDING、RUNNING、SUCCEEDED、PARTIAL_SUCCEEDED（扫描）、FAILED、CANCELLED；不输出 RequestJSON 或租约凭据。

summary 含 `total_return,annualized_return,maximum_drawdown,closed_trades,win_rate,profit_factor,average_holding_bars,has_open_position`；无定义的比例为 null。orders 含 `id,instrument,side,quantity,created_at,reason,attempted_at,final_reason`；trades 含 `id,order_id,instrument,side,time,price,quantity,gross,commission,stamp_duty,transfer_fee`；equity 含 `time,equity,cash,position_value`。时间统一 UTC，金额维持缩放整数；side=1买、2卖；final_reason 保持领域 OrderFinalReason 数值枚举（0待执行、1成交、2无下一根Bar）。

结果页共享 `?limit=100&after_sequence=0`：limit 默认100、范围1–1000，游标为非负 int64，重复分页参数拒绝。响应 `{items:[...],next_sequence:123}`；用服务端 next_sequence 继续，无此字段即结束。顺序来自持久化 sequence，不能用日期替代游标。

#### 扫描任务

```bash
curl -X POST http://localhost:8080/api/v1/scan-runs \
  -H 'Content-Type: application/json' \
  -d '{"strategy":"daily_b1_buy","strategy_version":"1","idempotency_key":"scan-example-1","from":"2026-01-01T00:00:00Z","as_of":"2026-06-01T00:00:00Z","scope":{"exchanges":["SSE","SZSE","BSE"],"active_only":true,"limit":5000}}'
```

`strategy,strategy_version,idempotency_key,from,as_of,scope` 必填，parameters 可选；scope.limit 为1–5000，exchanges 为空表示全部支持的交易所，active_only 缺省 false。创建锁定证券集合与 COMPLETE 数据版本。单证券失败允许 PARTIAL_SUCCEEDED，失败分类保存在快照。

`GET /api/v1/scan-runs/:run_id` 返回安全状态，成功/部分成功附 snapshot_id；`POST /api/v1/scan-runs/:run_id/cancel` 与回测取消一致。

#### 最新快照与续页

`GET /api/v1/signal-snapshots/latest?strategy=daily_b1_buy&limit=100`。仅给 strategy 选该策略最近发布快照（可跨版本/参数）；精确查询同时给 strategy_version、parameters_hash，可用 RFC3339 的 as_of 限定。仅 strategy 时，as_of 只对选中的最新快照作校验；历史定位应提供版本/hash 或 SnapshotID。

data 含 `snapshot_id,run_id,key,data_version,rows,failures` 和满页时的 next_sequence。key 含 `snapshot_id,strategy_id,strategy_version,parameters_hash,as_of`；rows 保留完整 instrument；failures 是按完整 instrument 排序的 `{instrument,code,message,retryable}` 数组，属于整个快照，不随成功行分页改变。

保存首响应的 key，再继续：

```text
/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&strategy_version=1&parameters_hash=HASH&snapshot_id=SNAPSHOT_ID&after_sequence=100&limit=100
```

after_sequence>0 必须带 snapshot_id。SnapshotID 仍需匹配策略、版本、参数 hash 和非零 as_of，新快照发布不会改变已开始的分页。扫描状态中的 snapshot_id 也可直接用于定位结果。

Task12 延后的游标输出在此补齐：不可变快照行以1开始连续编号，满页返回 `after_sequence+本页行数`。若末页恰好满页，允许下一请求返回空 rows，此时无 next_sequence；满页游标不保证还有数据。协议无须额外 COUNT，也不会重选快照。

#### 策略目录

`GET /api/v1/strategies` 返回按 ID、版本排序的已编译目录；`GET /api/v1/strategies/:strategy?version=1` 返回具体版本。含 `strategy,version,primary_timeframe,warmup_bars,default_hold_bars,parameters,features,auxiliary`，每个参数含 default/min/max/integer。内置版本1：daily_b1_buy、weekly_b1_buy、bottom_surge_pullback。未知策略/版本返回404。

#### 证券搜索

`GET /api/v1/instruments?q=海康&exchange=SZSE&limit=20` 只搜索活跃证券。`q` 必填且最多128字节：纯数字按六位代码前缀匹配，其他内容按证券名称包含匹配；`exchange` 可选 SSE、SZSE 或 BSE；`limit` 默认20、范围1–50。无匹配返回空 items。

每项包含完整 `instrument`、`code`、`name`、`exchange`、`board` 和 `lot_size`。搜索依赖 `t_instruments` 的名称和激活状态，上线前必须先完成证券主数据补齐。

#### 图表查询

`POST /api/v1/chart-queries` 在一个固定的 COMPLETE 行情版本上返回 K 线和技术指标：

```json
{
  "instrument": "SZSE:002415",
  "timeframe": "DAY",
  "price_view": "QFQ",
  "limit": 400,
  "data_version": 0,
  "indicators": [
    {"kind":"SMA","period":5},
    {"kind":"SMA","period":20},
    {"kind":"SMA","period":60}
  ]
}
```

`timeframe` 支持 DAY、WEEK；`price_view` 支持 RAW、QFQ；`limit` 默认400、范围100–1000。`data_version=0` 在首次请求解析最新版本，向前加载时必须回传响应中的正版本。`before` 是可选的 RFC3339 排他游标。

指标最多16个：SMA/EMA 使用1–500的 period；MACD 使用正数 fast/slow/signal 且 slow>fast；KDJ 使用1–500的 period。服务端还会按指标类型和周期执行总计算成本门禁，拒绝可能造成 CPU 放大的极端组合。响应的 series 按请求顺序返回，MACD 展开为 dif/dea/histogram，KDJ 展开为 k/d/j；预热期无效点不输出，客户端取消后会在指标计算边界停止。

响应 Bar 按 close_time 升序。`has_more` 表示当前游标之前、项目统一的20年查询边界内是否仍有数据；它不承诺提供20年以前的数据。`has_more=true` 时，使用 `next_before` 和相同 `data_version` 获取更早一页。服务先在完整历史上下文计算指标，再裁剪响应页，避免页边界指标跳变。

#### 查询版本化行情

`GET /api/v1/market/bars` 按完整证券身份读取已发布的 `COMPLETE` 行情版本，股票和期货共用同一接口。

```bash
curl 'http://localhost:8080/api/v1/market/bars?instrument=SHFE%3AAU.MAIN&timeframe=daily&view=raw&from=2026-01-01&to=2026-09-14&limit=100'
```

| 参数 | 必填 | 缺省值 | 说明 |
|---|---|---|---|
| instrument | 是 | - | 完整身份，例如 `SSE:600000`、`SHFE:AU.MAIN` |
| timeframe | 否 | daily | `daily` 或本地聚合的 `weekly` |
| view | 否 | raw | `raw` 或 `qfq`；期货当前使用 1:1 因子，两者相同 |
| from / to | 否 | 最近19年 | `YYYY-MM-DD`，闭区间，最大20年 |
| version | 否 | 0 | 0 表示读取最新 `COMPLETE` 版本 |
| limit | 否 | 100 | 1–5000 |

响应 data 含 `instrument,timeframe,view,data_version,bars`。未知参数、重复参数、非法枚举或超限范围返回400。当前期货只采集新浪定义的八条主力连续序列：`SHFE:AU.MAIN`、`SHFE:AG.MAIN`、`SHFE:FU.MAIN`、`INE:SC.MAIN`、`INE:LU.MAIN`、`DCE:J.MAIN`、`DCE:JM.MAIN`、`CZCE:ZC.MAIN`。它们不是可交割合约，系统不自行换月；`ZC.MAIN` 上游历史目前停在 2022-12-30，不能当作仍在更新的实时序列。

### 1. 保存股票历史数据（兼容路径）

按六位代码从活跃证券主数据中精确解析证券，再通过新浪获取原始日线和前复权因子，周线在本地由日线确定性聚合。所有数据校验通过后一次原子发布新的 `COMPLETE` 版本；不会写旧 K 线表。

- **Method**: `POST`
- **Path**: `/api/stocks/historical`
- **Content-Type**: `application/json`

#### 请求参数

| 字段 | 类型   | 必填 | 说明                     |
|------|--------|------|--------------------------|
| code | string | 是   | 六位股票代码，如 `600312` |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/stocks/historical \
  -H "Content-Type: application/json" \
  -d '{"code": "600312"}'
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "instrument": {"Exchange":"SSE","Code":"600312"},
    "version": 12,
    "quality": "COMPLETE",
    "daily_bars": 4800,
    "weekly_bars": 960
  }
}
```

无活跃证券返回404；同一代码落在多个交易所返回409；同证券正在刷新返回429。请求 JSON 限制1MiB，拒绝未知字段和尾随 JSON。

---

### 2. 补全股票数据（兼容路径）

异步触发一次全市场版本化行情刷新。证券集合来自 `t_instruments` 中的活跃记录，使用配置的 `Worker.ScanBatchSize` 有界并发；同一进程内定时刷新和手工刷新只允许一个运行实例。

- **Method**: `POST`
- **Path**: `/api/stocks/append`
- **说明**: 异步执行，同一时间只能执行一个任务

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/stocks/append
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {"status":"ACCEPTED"}
}
```

成功 HTTP 状态为202；已有刷新运行时返回429、`MARKET_REFRESH_ALREADY_RUNNING`。任务使用应用根 context，HTTP 请求结束不会终止已接受的刷新，服务停机时会取消并等待退出。

---

### 3. 保存股票财报数据

从行情数据源获取指定股票近5年（20份季度）的财报数据，写入数据库。

- **Method**: `POST`
- **Path**: `/api/stocks/financial-report`
- **Content-Type**: `application/json`

#### 请求参数

| 字段 | 类型   | 必填 | 说明                     |
|------|--------|------|--------------------------|
| code | string | 是   | 股票代码，如 `600312`    |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/stocks/financial-report \
  -H "Content-Type: application/json" \
  -d '{"code": "600312"}'
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

### 4. 最新股票买点快照（兼容路径）

- **Method**: `GET`
- **Path**: `/api/stocks/signal?strategy=daily_b1_buy`

只读该策略最近已发布的成功或部分成功快照，不启动扫描。旧路径无法表达参数或版本，默认跨参数、跨策略版本按 `as_of DESC, data_version DESC, id DESC` 选取一条；随后所有分页固定其 SnapshotID。返回完整代码集合，只有此处去掉交易所：

```json
{"code":0,"message":"success","data":{"name":"daily_b1_buy","codes":["600000","000001"]}}
```

已发布但无信号时 `codes:[]`；没有已发布快照返回 HTTP 409、`message:"SIGNAL_SNAPSHOT_NOT_READY"`；未知策略返回404。需要精确参数、版本、失败证券信息时使用下文 V1 快照接口。已移除旧短线/长线评分输出。

### 5. 策略回测（兼容路径）

- **Method**: `GET`
- **Path**: `/api/stocks/backtest?code=600000&strategy=daily_b1_buy`
- `code` 必须是六位数字；仅从 `t_instruments` 精确查找活跃证券，不依据前缀猜交易所。无匹配返回404，多交易所匹配返回409、`AMBIGUOUS_INSTRUMENT`。
- `strategy` 必填，使用内置版本1；可选 `cycle=daily|weekly` 必须与策略主周期一致，否则400。

每次 GET 生成新幂等键并创建持久化 Run。需要重试复用同一任务时，请使用 V1 创建接口的显式 `idempotency_key`，或直接查询已经返回的 `run_id`。

默认回测最近10年到请求时 UTC 时刻，初始资金100万元、100%现金投入、佣金3bps/最低5元、卖出印花税5bps、过户费0、滑点5bps、整手100股，持有期使用策略默认值。这些是兼容路径的固定执行假设，不代表当前市场费率；自定义配置使用 V1。

生产最多同步等待2秒（装配硬上限30秒），请求断开只结束等待。任务由后台持久化 worker 继续执行；未完成返回202：

```json
{"code":0,"message":"success","data":{"run_id":"...","status":"PENDING"}}
```

窗口内完成返回200，`data` 包含 `code,strategy,cycle,run_id,status,summary,orders,trades,equity`，字段与 V1 结果一致。完整响应只从已原子发布的结果读取，等待和分页共用时间预算。任务失败或取消返回409、`RUN_RESULT_NOT_READY`。旧的“信号日期数组”已替换为成交、权益和汇总结果。

---

### 6. 查询股价 K 线数据（兼容路径）

从指定或最新 `COMPLETE` 行情版本查询日线/周线，支持原始价和前复权价。六位代码只匹配活跃证券；不回退旧 K 线表。

- **Method**: `GET`
- **Path**: `/api/stocks/price`

#### 请求参数

| 字段     | 类型   | 必填 | 默认值   | 说明                                 |
|----------|--------|------|----------|--------------------------------------|
| code     | string | 是   | -        | 股票代码，如 `600312`               |
| cycle    | string | 否   | `daily`  | 周期：`daily`（日线）或 `weekly`（周线）|
| pagesize | int    | 否   | `20`     | 每页条数，1–1000                     |
| pagenum  | int    | 否   | `1`      | 页码，从1开始，`pagesize*pagenum` 最大5000 |
| view     | string | 否   | `raw`    | `raw` 原始价或 `qfq` 前复权价        |
| version  | uint64 | 否   | `0`      | 0 使用最新 COMPLETE 版本，正数锁定版本 |

#### 请求示例

```bash
# 查询日线数据（默认分页）
curl "http://localhost:8080/api/stocks/price?code=600312"

# 查询周线数据，每页 10 条，第 2 页
curl "http://localhost:8080/api/stocks/price?code=600312&cycle=weekly&pagesize=10&pagenum=2"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "code": "600312",
    "cycle": "daily",
    "view": "raw",
    "data_version": 12,
    "data": [
      {
        "open_time": "2022-01-22T00:00:00Z",
        "close_time": "2022-01-22T00:00:00Z",
        "open": 10.5,
        "high": 11.2,
        "low": 10.3,
        "close": 10.8,
        "volume": 1234567,
        "amount": 13500000,
        "trading_status": 0
      }
    ]
  }
}
```

#### 响应字段说明

| 字段     | 类型     | 说明                   |
|----------|----------|------------------------|
| code     | string   | 股票代码               |
| cycle    | string   | 数据周期：daily 或 weekly |
| view     | string   | raw 或 qfq              |
| data_version | uint64 | 本次读取的不可变行情版本 |
| data     | []object | K 线数据列表，按收盘时间升序 |

**data 数组元素字段：**

| 字段   | 类型    | 说明           |
|--------|---------|----------------|
| open_time / close_time | RFC3339 | UTC Bar 时间 |
| open / high / low / close | float64 | 按 view 转换后的价格 |
| volume | int64 | 成交量 |
| amount | float64 | 成交额 |
| trading_status | uint8 | 0 可交易、1 停牌 |

---

### 7. 补全财报数据

手动触发财报数据补全扫描，检查所有股票代码的财报数据完整性，自动补充缺失的季度财报。

- **Method**: `POST`
- **Path**: `/api/stocks/financial-report/append`
- **说明**: 异步执行，同一时间只能执行一个任务

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/stocks/financial-report/append
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

### 8. 查询财报数据

根据股票代码查询季度财报数据，支持分页。

- **Method**: `GET`
- **Path**: `/api/stocks/financial-report`

#### 请求参数

| 字段     | 类型   | 必填 | 默认值   | 说明                                 |
|----------|--------|------|----------|--------------------------------------|
| code     | string | 是   | -        | 股票代码，如 `600312`               |
| pagesize | int    | 否   | `20`     | 每页条数                             |
| pagenum  | int    | 否   | `1`      | 页码，从 1 开始                      |

#### 请求示例

```bash
# 查询财报数据（默认分页）
curl "http://localhost:8080/api/stocks/financial-report?code=600312"

# 每页 5 条，第 2 页
curl "http://localhost:8080/api/stocks/financial-report?code=600312&pagesize=5&pagenum=2"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "code": "600312",
    "data": [
      {
        "code": "600312",
        "report_date": "20250930",
        "report_type": 4,
        "total_revenue": 300000000000.0000,
        "total_cost": 250000000000.0000,
        "net_profit": 15000000000.0000,
        "net_profit_cut": 14000000000.0000,
        "gross_margin": 0.1667,
        "net_margin": 0.0500,
        "operating_margin": 0.0667,
        "ebit_margin": 0.0700,
        "cost_profit_ratio": 0.0800,
        "roe": 0.1200,
        "roa": 0.0800,
        "asset_liability_ratio": 0.4500,
        "current_ratio": 1.5000,
        "quick_ratio": 1.2000,
        "total_asset_turnover": 0.6000,
        "inventory_turnover": 4.0000,
        "receivables_turnover": 8.0000,
        "operating_cash_flow": 20000000000.0000,
        "operating_cash_flow_per_share": 2.5000,
        "eps": 1.8000,
        "bps": 15.0000
      }
    ]
  }
}
```

#### 响应字段说明

| 字段     | 类型     | 说明                   |
|----------|----------|------------------------|
| code     | string   | 股票代码               |
| data     | []object | 财报数据列表           |

**data 数组元素字段：**

| 字段                          | 类型    | 说明                       |
|-------------------------------|---------|----------------------------|
| code                          | string  | 股票代码                   |
| report_date                   | string  | 报告期，格式 YYYYMMDD      |
| report_type                   | int     | 报告类型：1一季报 2半年报 3三季报 4年报 |
| total_revenue                 | float64 | 营业总收入                 |
| total_cost                    | float64 | 营业成本                   |
| net_profit                    | float64 | 归母净利润                 |
| net_profit_cut                | float64 | 扣非净利润                 |
| gross_margin                  | float64 | 毛利率                     |
| net_margin                    | float64 | 销售净利率                 |
| operating_margin              | float64 | 营业利润率                 |
| ebit_margin                   | float64 | 息税前利润率               |
| cost_profit_ratio             | float64 | 成本费用利润率             |
| roe                           | float64 | 净资产收益率               |
| roa                           | float64 | 总资产报酬率               |
| asset_liability_ratio         | float64 | 资产负债率                 |
| current_ratio                 | float64 | 流动比率                   |
| quick_ratio                   | float64 | 速动比率                   |
| total_asset_turnover          | float64 | 总资产周转率               |
| inventory_turnover            | float64 | 存货周转率                 |
| receivables_turnover          | float64 | 应收账款周转率             |
| operating_cash_flow           | float64 | 经营现金流量净额           |
| operating_cash_flow_per_share | float64 | 每股经营现金流             |
| eps                           | float64 | 基本每股收益               |
| bps                           | float64 | 每股净资产                 |

---

### 9. 财报信号扫描

扫描数据库中所有有财报数据的股票，筛选出连续多个季度净利润同比增长超过指定阈值的股票。

- **Method**: `GET`
- **Path**: `/api/stocks/financial-report/signal`
- **说明**: 需要扫描数据库，耗时较长，建议超时时间 30s

#### 请求参数

| 字段              | 类型    | 必填 | 默认值 | 说明                                    |
|-------------------|---------|------|--------|-----------------------------------------|
| profit_threshold  | float64 | 否   | `0.1`  | 净利润最低同比增长率，如 `0.1` 表示 10%  |
| quarter_count     | int     | 否   | `4`    | 需要连续满足的季度数                    |

#### 请求示例

```bash
# 默认参数：连续 4 个季度净利润同比增长 >= 10%
curl "http://localhost:8080/api/stocks/financial-report/signal"

# 自定义参数：连续 3 个季度净利润同比增长 >= 15%
curl "http://localhost:8080/api/stocks/financial-report/signal?profit_threshold=0.15&quarter_count=3"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "strategy": "financial_profit_growth",
    "profit_threshold": 0.1,
    "quarter_count": 4,
    "codes": ["600312", "000001"]
  }
}
```

#### 响应字段说明

| 字段             | 类型     | 说明                           |
|------------------|----------|--------------------------------|
| strategy         | string   | 策略名称                       |
| profit_threshold | float64  | 净利润同比增长率阈值           |
| quarter_count    | int      | 连续季度数                     |
| codes            | []string | 符合条件的股票代码列表         |

---

### 10. 查询 Shibor 利率

获取 Shibor 利率数据，支持按期限筛选。

- **Method**: `GET`
- **Path**: `/api/macro/shibor`

#### 请求参数

| 字段   | 类型   | 必填 | 默认值 | 说明                          |
|--------|--------|------|--------|-------------------------------|
| period | string | 否   | -      | 期限ID：001=隔夜, 002=1周, 003=2周, 004=1月, 005=3月, 006=6月, 007=9月, 008=1年；不传返回所有期限 |

#### 请求示例

```bash
# 查询隔夜 Shibor
curl "http://localhost:8080/api/macro/shibor?period=001"

# 查询所有期限
curl "http://localhost:8080/api/macro/shibor"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "period": "001",
    "data": [
      {
        "report_date": "2026-05-12",
        "report_period": "隔夜(O/N)",
        "ir_rate": 1.2380,
        "change_rate": -3.30,
        "indicator_id": "001"
      }
    ]
  }
}
```

#### 响应字段说明

| 字段          | 类型    | 说明           |
|---------------|---------|----------------|
| period        | string  | 请求的期限ID   |
| data          | []object| Shibor 数据列表 |

**data 数组元素字段：**

| 字段           | 类型    | 说明               |
|----------------|---------|--------------------|
| report_date    | string  | 报告日期           |
| report_period  | string  | 期限描述           |
| ir_rate        | float64 | 利率值             |
| change_rate    | float64 | 变化点数（基点）   |
| indicator_id   | string  | 指标ID             |

---

### 11. 查询汇率

获取汇率实时数据，支持按代码筛选。

- **Method**: `GET`
- **Path**: `/api/macro/exchange-rate`

#### 请求参数

| 字段 | 类型   | 必填 | 默认值 | 说明                              |
|------|--------|------|--------|-----------------------------------|
| code | string | 否   | -      | 汇率代码；不传返回所有预设汇率 |

**常见汇率代码：**

| 代码     | 说明        |
|----------|-------------|
| USDCNY   | 美元/人民币 |
| USDJPY   | 美元/日元   |
| DINIW    | 美元指数    |
| EURUSD   | 欧元/美元   |
| GBPUSD   | 英镑/美元   |
| USDCNH   | 美元/离岸人民币 |
| AUDUSD   | 澳元/美元   |
| USDCAD   | 美元/加元   |

#### 请求示例

```bash
# 查询美元/人民币汇率
curl "http://localhost:8080/api/macro/exchange-rate?code=USDCNY"

# 查询所有预设汇率
curl "http://localhost:8080/api/macro/exchange-rate"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "code": "USDCNY",
    "data": [
      {
        "code": "USDCNY",
        "name": "",
        "open": 7.2000,
        "now": 7.2150,
        "change_percent": 0.21
      }
    ]
  }
}
```

#### 响应字段说明

| 字段     | 类型     | 说明           |
|----------|----------|----------------|
| code     | string   | 请求的汇率代码 |
| data     | []object | 汇率数据列表   |

**data 数组元素字段：**

| 字段           | 类型    | 说明               |
|----------------|---------|--------------------|
| code           | string  | 汇率代码           |
| name           | string  | 显示名称           |
| open           | float64 | 开盘价             |
| now            | float64 | 最新价             |
| change_percent | float64 | 涨跌幅（百分比）   |

---
