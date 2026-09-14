# 新浪股票与商品期货日线接入设计

## 1. 决策摘要

- A 股继续使用新浪：原始日线 + qfq 因子；所有请求全局最小间隔 5 秒，周线本地聚合。
- 商品期货只接一个已实测可用的新浪日线接口，不引入 AKShare/Python sidecar，也不分别对接四家交易所。
- 期货第一阶段只保存八个新浪主力连续序列：AU、AG、FU、SC、LU、J、JM、ZC。
- 不采集真实交割合约，不自建主力选择和换月规则。新浪连续序列按原样作为 `*.MAIN` 保存。
- 股票与期货复用现有版本化行情、日周 Bar、任务和查询架构；允许停机升级，不建设双写或在线迁移。

## 2. 背景与数据源结论

东方财富 `push2his.eastmoney.com` 在低频单请求下仍直接断开连接。相同环境访问 `push2.eastmoney.com` 和 `datacenter-web.eastmoney.com` 正常，增加常见 header、`ut` 参数以及绕过代理均不能恢复。因此当前问题更接近域名或出口 IP 的连接级拒绝，而不是可以通过普通 sleep 解决的 `429` 限频。

新浪股票接口已验证每 5 秒一次可稳定使用，因此保留为 A 股源。新浪期货 `InnerFuturesNewService.getDailyKLine` 在 2026-09-14 实测以下符号均为 HTTP 200，并一次返回完整历史：

| Instrument | 新浪符号 | 品种 |
|---|---|---|
| `SHFE:AU.MAIN` | `AU0` | 黄金 |
| `SHFE:AG.MAIN` | `AG0` | 白银 |
| `SHFE:FU.MAIN` | `FU0` | 燃料油 |
| `INE:SC.MAIN` | `SC0` | 原油 |
| `INE:LU.MAIN` | `LU0` | 低硫燃料油 |
| `DCE:J.MAIN` | `J0` | 焦炭 |
| `DCE:JM.MAIN` | `JM0` | 焦煤 |
| `CZCE:ZC.MAIN` | `ZC0` | 动力煤 |

实测 AU0 返回 4551 根、J0 返回 3743 根。ZC0 最后日期为 2022-12-30，应视为该连续品种的数据现状，不用未来数据或其他品种填补。

四家交易所也各自有日行情接口，但并非一个统一 API：SHFE、INE 和 CZCE 的公开接口可直接访问；DCE 的 AKShare 旧端点当前返回 412，新免费官方 API 需要注册 token。既然当前目标只是获得这些商品的日线，逐交易所适配与真实合约换月不值得增加部署和维护成本。

## 3. 目标与非目标

### 目标

1. 股票生产行情切换到新浪原始日线与 qfq 因子。
2. 保存八个商品期货主力连续品种的日线与本地周线。
3. 股票与期货都通过原子 `PENDING -> COMPLETE` 版本发布，查询固定版本可复现。
4. 免费数据源异常时拒绝坏数据并保留上一完整版本。
5. 总测试覆盖率保持 80% 以上。

### 非目标

- Tushare、东方财富股票历史行情自动 fallback。
- AKShare 运行时、Python sidecar、四家交易所分别适配。
- 真实期货合约、合约元数据、主力选择、换月映射、连续复权因子。
- 期货结算、保证金、成交额、交割、期货撮合和专用回测模型。
- 分钟线、Tick、盘口、实时行情、Redis、Kafka 或分布式采集。

## 4. 架构

```text
新浪股票日线 + qfq -> SinaMarketSource ----+
                                                |
新浪期货主力日线 ----> SinaFuturesSource --+--> MarketIngestionService
                                                |    ├── 日线合并
                                                |    ├── 本地周线聚合
                                                |    └── 单位/股票 qfq 因子
                                                v
                                      MarketDataRepository
                         t_instruments / versions / market_bars
                                                |
                                                v
                                    GET /api/v1/market/bars
```

依赖方向保持 `api/pkg/infrastructure -> application -> port/market`。外部 JSONP 只在 `pkg/broker` 解析；领域与应用层不知道 URL 或新浪字段名。

## 5. 股票链路

### 5.1 协议

原始日线：

```text
https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/
CN_MarketData.getKLineData?symbol=sh600000&scale=240&ma=no&datalen=N
```

前复权因子：

```text
https://finance.sina.com.cn/realstock/company/sh600000/qfq.js
```

只接受 `day/open/high/low/close/volume`。`qfq.js` 的 `f` 表示 `qfq = raw / f`，通过 `big.Rat` 从十进制字符串直接生成约分因子。首次最多请求 10000 根；增量同步回看最近 20 根，旧于请求起点的上游 lookback 行过滤，晚于请求终点的行拒绝。

### 5.2 周线与发布

合并完整日线后按 ISO 周聚合：首日 open、最高 high、最低 low、末日 close、volume/amount 求和，周 Bar 时间取实际最后交易日。当前 ISO 周暂不发布，到进入下一 ISO 周后再发布最终 Bar，避免周内每天产生不同 close_time 的重复周线。日线、已完结周线和完整因子在一个 `MarketWriteBatch` 内原子发布。

## 6. 期货链路

### 6.1 新浪协议

```text
https://stock2.finance.sina.com.cn/futures/api/jsonp.php/
var%20_<变量>=/InnerFuturesNewService.getDailyKLine?symbol=AU0&type=2026_9_14
```

响应字段为：

- `d`：交易日期；
- `o/h/l/c`：OHLC；
- `v`：成交量；
- `p`：持仓量；
- `s`：结算价。

第一阶段持久化 OHLCV。`p/s` 只做非负数值校验，避免字段漂移被误当成功；现有通用 Bar 没有持仓量与结算价，且当前需求只需要日线，所以不新增期货专用表。新浪没有提供该序列的成交额，`Bar.Amount` 保存为 0。

### 6.2 身份与语义

期货连续 ID 使用交易所限定的规范形式，例如 `SHFE:AU.MAIN`。代码必须是大写，交易所与品种必须匹配首批白名单。

`.MAIN` 明确表示“新浪提供的主力连续序列”，不是项目根据真实合约重新计算的 point-in-time 主力。数据版本能复现每次抓取的 Bar 快照，但不能解释新浪历史换月选择；若未来需要可审计的真实合约回测，再另立需求接交易所数据与换月规则。

期货不需要前复权，adapter 返回从 2000-01-01 生效的 1:1 单位因子，使现有 ingestion、查询和指标路径无需平行实现。新浪接口本身每次返回完整历史，因此期货刷新从配置的历史起点重读，以观察早期修订；股票仍只重读最近 20 根。

## 7. 限频、重试与错误

- 股票日线、股票 qfq 和期货主力日线共用一个进程级 `rate.Limiter`：默认 `rate.Every(5*time.Second)`、burst 1。
- 每只股票正常 2 次请求；每轮期货正常 8 次请求，单独完成理论下限约 40 秒。
- 每次重试重新等待 limiter。`429/Retry-After` 采用不小于服务端要求的等待；超时和连接重置最多三次。
- 响应上限 4 MiB，严格检查唯一 JSON/JSONP 值、日期、重复、OHLC、成交量、持仓量和结算价。
- 错误不包含响应正文、Cookie、完整 URL 或配置；失败不发布空版本。

## 8. 调度与配置

```yaml
Config:
  Market:
    StockRequestIntervalSeconds: 5
    FuturesEnabled: true
    FuturesRefreshIntervalHours: 24
```

股票 scheduler 只枚举 SSE、SZSE、BSE，避免把期货 Instrument 发送给股票 adapter。期货 scheduler 使用固定八个 ID，按稳定顺序逐个刷新；单品种失败记录在汇总并继续，取消时停止后续请求。默认启动时立即执行，随后每 24 小时执行。

这是单进程个人项目，不增加分布式锁或持久化采集任务。相同内容由 digest 判定为 no-op。

## 9. 查询

新增：

```text
GET /api/v1/market/bars?instrument=SHFE:AU.MAIN&timeframe=daily&view=raw&limit=100
```

支持 `daily/weekly`、`raw/qfq`、`from/to`、`version` 和 `limit`。期货 raw 与 qfq 相同，因为单位因子为 1:1。请求使用完整 Instrument ID，严格拒绝重复或未知参数。

## 10. 数据库与停机更新

复用：

- `t_instruments`：只把 code 从 6 扩展到 16 字符以容纳 `.MAIN`；资产类别、连续类型和品种由规范 Instrument ID 确定，不预建真实合约元数据；
- `t_market_data_versions`：全局版本与完成状态；
- `t_market_bars`：股票与期货日/周 Bar。

不增加期货专用表。上线采用停服务、备份、迁移、校验、启动流程，不建设双写、影子表或跨版本兼容状态机。

## 11. 测试与验收

- 新浪股票日线/qfq 解析、5 秒共享节拍、本地周线和版本发布测试。
- 新浪期货八个固定符号、JSONP、十进制定点、重复/坏值/超限/取消测试。
- 期货 scheduler 顺序、部分失败、重复运行和取消测试。
- 通用行情 API 的规范 ID、严格参数、版本和 daily/weekly 测试。
- MySQL 5.7/8.0 验证 Instrument schema 与版本发布；总体覆盖率大于 80%。

验收时用八个符号做只读在线 smoke test，但普通单元测试不访问公网。完整交付执行 `bash scripts/verify.sh`。

## 12. 外部依据

- [AKShare 1.18.94 新浪期货实现](https://github.com/akfamily/akshare/blob/release-v1.18.94/akshare/futures/futures_zh_sina.py)
- [AKShare 期货接口文档](https://akshare.akfamily.xyz/data/futures/futures.html)
- [DCE 旧接口失效记录](https://github.com/akfamily/akshare/issues/7252)

公开接口可能变化。固定 fixture 用于发现契约变化，线上 HTTP 200 不能替代内容校验。
