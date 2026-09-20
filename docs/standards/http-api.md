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
| 409 | AMBIGUOUS_INSTRUMENT | 六位代码匹配多个活跃交易所证券 |
| 429 | MARKET_REFRESH_ALREADY_RUNNING | 已有定时或手工行情刷新运行中（全市场或同一证券） |
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

#### 自选清单

`GET /api/v1/watchlist` 返回按添加时间升序的自选证券。每项包含完整身份（`instrument,code,name,exchange,board,lot_size`）与最新日线报价 `close,change,change_pct`：价格以元为单位，`change_pct` 为百分数原值；无可用数据时相应字段为 `null`。仅返回当前活跃证券，非活跃条目保留在表中但不显示。

`POST /api/v1/watchlist` 请求体 `{"instrument":"SSE:600000"}`；身份非法 400，未知或非活跃证券 404，已存在视为幂等成功，上限 100 只、超出 409（message 为 `WATCHLIST_FULL`）。`DELETE /api/v1/watchlist/:instrument` 完整身份按 URL 编码传递（如 `SSE%3A600000`），条目不存在视为幂等成功。两个变更接口成功后返回更新后的完整列表。

```bash
curl http://localhost:8080/api/v1/watchlist
curl -X POST http://localhost:8080/api/v1/watchlist \
  -H 'Content-Type: application/json' \
  -d '{"instrument":"SSE:600000"}'
curl -X DELETE http://localhost:8080/api/v1/watchlist/SSE%3A600000
```

报价取最新 `COMPLETE` 数据版本的当前（未失效）日线 bar，`change` 与 `change_pct` 基于上一根日线收盘价；不足两根 bar 时相应字段为 `null`。POST 请求 JSON 限制 1MiB，拒绝未知字段和尾随第二个 JSON 值。

#### 行情看板

看板配置单用户全局持久化于 `t_chart_boards`（最多 20 个），五个接口成功后均返回更新后的全量状态 `{"boards":[{"id":1,"name":"默认看板","config":{...}}...],"active_id":1}`，boards 按 id 升序。`config` 的 JSON 字段名与前端 `BoardConfig` 一致（defaultSymbol、timeframe、priceView、indicators、comparison、paneWeights、visibleBars），服务端写路径严格校验后规范化落库、读路径原样透传。

- `GET /api/v1/chart-boards` 返回全量状态。
- `POST /api/v1/chart-boards` 请求体 `{"name":"...","config":{...}}`，创建即激活。
- `PUT /api/v1/chart-boards/:id` 请求体 `{"name":"..."} `或 `{"config":{...}}`（至少一项），不触碰激活状态。
- `POST /api/v1/chart-boards/:id/activate` 激活指定看板（恰一不变量由服务端维护）。
- `DELETE /api/v1/chart-boards/:id` 删除指定看板；删除激活看板时服务端激活剩余 id 最小者。

校验与错误映射：名称 1–40 字符（首尾空白剔除）；config 校验与图表查询同口径（defaultSymbol 为空或 `^(SSE|SZSE|BSE):[A-Z0-9]{1,32}$`、timeframe/priceView 枚举、指标 ≤16 且身份唯一、comparison 为空或八项期货白名单之一、visibleBars 10–400 整数、paneWeights ≤18 键且值 ∈ (0,10000]）；JSON/字段/校验错误 400 INVALID_REQUEST，未知 id 404 NOT_FOUND，超上限 409 BOARDS_FULL，删除最后一块 409 LAST_BOARD。请求体上限 1MiB，拒绝未知字段与尾随 JSON（嵌套 config 与指标对象递归适用）。

```bash
curl http://localhost:8080/api/v1/chart-boards
curl -X POST http://localhost:8080/api/v1/chart-boards \
  -H 'Content-Type: application/json' \
  -d '{"name":"默认看板","config":{"defaultSymbol":null,"timeframe":"DAY","priceView":"QFQ","indicators":[{"kind":"SMA","period":5}],"comparison":null,"paneWeights":{},"visibleBars":120}}'
curl -X PUT http://localhost:8080/api/v1/chart-boards/1 \
  -H 'Content-Type: application/json' \
  -d '{"config":{"defaultSymbol":"SSE:600000","timeframe":"DAY","priceView":"QFQ","indicators":[],"comparison":null,"paneWeights":{},"visibleBars":120}}'
curl -X POST http://localhost:8080/api/v1/chart-boards/2/activate
curl -X DELETE http://localhost:8080/api/v1/chart-boards/2
```

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

指标最多16个：SMA/EMA/STD 使用1–500的 period；MACD 使用正数 fast/slow/signal 且 slow>fast；KDJ 使用1–500的 period。服务端还会按指标类型和周期执行总计算成本门禁，拒绝可能造成 CPU 放大的极端组合。STD 对当前价格视图收盘价计算滚动总体标准差（分母为 period，常量窗口为0），成本按 period 计入2000预算，返回单条 value 序列，预热不足不输出；在完整历史计算后裁页。响应的 series 按请求顺序返回，MACD 展开为 dif/dea/histogram，KDJ 展开为 k/d/j；预热期无效点不输出，客户端取消后会在指标计算边界停止。

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

#### 手动刷新行情

`POST /api/v1/market/refresh`：请求体缺省时异步触发一次全市场版本化行情刷新；提供 `exchange`+`code` 时同步刷新单只证券。

```bash
# 全量刷新（异步，202）
curl -X POST http://localhost:8080/api/v1/market/refresh

# 单证券刷新（同步，200）
curl -X POST http://localhost:8080/api/v1/market/refresh \
  -H 'Content-Type: application/json' \
  -d '{"exchange":"SSE","code":"600312"}'
```

- 请求体可选：`exchange` 与 `code` 必须同时提供或同时缺省，任一单独出现返回400。
- 全量触发：语义与调度器定时刷新一致；同一进程内定时与手工刷新只允许一个运行实例，运行中返回429；成功受理返回 `202 {"status":"ACCEPTED"}`。
- 单证券：`exchange` 合法值为 SSE、SZSE、BSE，`code` 为六位数字；经活跃证券主数据精确解析，零结果或唯一匹配的交易所与请求身份不符返回404，同一代码匹配多个活跃交易所返回409，同一证券正在刷新返回429。成功返回 `{"instrument","version","quality","daily_bars","weekly_bars"}`，与既有刷新结果结构一致。
- 请求 JSON 限制1MiB，拒绝未知字段和尾随第二个 JSON 值。

