---
status: approved
authority: normative
baseline_revision: 8693e59
approval_provenance: inherited-current-design
approved_by: null
approved_at: null
approved_revision: null
owns: ["pkg/broker/", "pkg/broker/testdata/", "pkg/indicator/limiter.go", "pkg/indicator/limiter_test.go"]
related: []
---

# 外部数据源适配设计

| 属性 | 内容 |
|---|---|
| 状态 | 当前有效 |
| 适用范围 | `pkg/broker`、共享 `pkg/indicator.Limiter` |
| 最后更新 | 2026-09-14 |

## 1. 职责与非职责

本模块把新浪和东方财富等外部 HTTP 响应转换为经过边界校验的行情、复权因子、公司行动、财报和宏观数据。新行情内核通过 `internal/port` 接口消费适配器；旧财报业务继续使用兼容 Broker 能力。

Broker 不选择生产行情版本、不写数据库、不聚合生产周线或决定策略信号。新行情与迁移适配器必须把来源错误和不完整数据显式返回；旧 `SinaBroker` 的宽松解析例外见第 5 节。

## 2. 数据源与能力

- `SinaMarketSource`：A 股日线与前复权因子，支持 SSE、SZSE、BSE 完整身份。
- `SinaFuturesSource`：固定期货 `.MAIN` 主力连续日线；向领域提供 1:1 因子。
- `EastmoneyMarketSource`：原始/QFQ K 线和公司行动，主要服务旧迁移回填与兼容能力。
- 其他新浪/东方财富入口：财报、汇率、SHIBOR 和旧业务查询。

生产行情来源选择和刷新窗口由应用层定义。

## 3. 依赖和边界

- 外部 symbol、JSON/JSONP、数字字符串、日期和分页元数据在本模块解析，领域层不接触来源 DTO。
- 构造 `market.Bar` 后再次通过 Dataset/领域校验验证 OHLC、时间和所有权。
- 新行情来源适配器的 HTTP 客户端具有显式超时、响应体上限和 context；测试可注入 fixture server 与基础 URL。
- 新股票和期货来源适配器共享同一进程级 `rate.Limiter`，生产配置间隔不得低于 5 秒，禁止以并发绕过来源限制。
- 限频器约束的是 HTTP 请求获得令牌并开始执行的时刻。上一个响应的处理耗时会发生在两次 `Wait` 之间，因此测试必须在 `RoundTripper` 请求入口观测放行时间，不能用测试服务器收到请求的时间差替代该契约。

## 4. 核心不变量

### 4.1 新浪股票

- 完整身份确定 `sh`、`sz` 或 `bj` symbol，未知交易所拒绝。
- 日线请求长度有 30–10000 的边界；返回日期去重、不得超出请求上界，空响应视为不完整。
- 价格按十进制定点解析为缩放整数，不通过 binary float 中转。
- QFQ 因子必须为正并覆盖所需日期；缺因子不能生成看似完整的复权行情。

### 4.2 新浪期货

- 只接受领域支持且在生产白名单中的 `.MAIN` 身份。
- 当前白名单为 AU、AG、FU、SC、LU、J、JM、ZC 对应的八条主力连续序列。
- JSONP 变量名必须与请求 symbol 和日期令牌匹配，附加值、重复日期、非法 OHLC/成交量/元数据全部拒绝。
- 上游返回完整历史快照；应用层每次从配置历史起点读取以发现早期修订。
- 连续序列不被解释为具体合约，系统不自行换月；当前使用 1:1 调整因子。

### 4.3 东方财富

- 原始与 QFQ K 线按日期精确对齐后推导因子，任一缺页、漂移或冲突拒绝整个结果。
- 公司行动分页的 count/pages/pageNumber 必须稳定，记录不得重复，最终接收数量等于声明数量。
- 来源明确返回合法 no-data 时允许空公司行动；分页中途 no-data 是不完整数据。

## 5. 请求、重试与失败

- context 取消立即停止等待、请求或分页。
- 新行情来源把响应状态、超时、取消、畸形和不完整数据映射为稳定 Broker 错误；应用层据此决定是否重试。
- 新新浪行情来源传输最多进行有界重试，只重试超时、429 和 5xx，并尊重合法 `Retry-After`；取消不重试。
- 新行情与迁移来源若不能证明响应完整，就返回错误而不是部分 Bar/Action。

### 5.1 旧兼容 `SinaBroker`

`SinaBroker` 服务财报、宏观、旧实时行情等兼容调用，保留独立 HTTP 行为：15 秒客户端超时、最多 3 次线性退避重试；传输错误、读体错误和 5xx 会重试，4xx（包括 429）直接失败，不解释 `Retry-After`。它不接入新行情来源的共享 `rate.Limiter`，当前读体也没有硬上限。

旧实时行情批量解析会跳过字段不足或无法识别的记录并返回其余结果；股票数值解析失败时沿用零值，缺失日期使用本机当前日期。除单证券查询最终找不到代码外，这些情况不会转换为批次级错误，因此调用方可能收到部分批次或含零值的记录。

这些规则是现状兼容边界，不是新适配器范式。统一限频、错误分类和响应上限可能改变旧调用的失败及重试语义，必须通过大型需求推进；新代码不得复制该例外。

## 6. 安全与性能约束

- 新适配器的 URL 基址必须能解析为合法 scheme/host，但附加部分按端点处理：新浪期货和因子基址拒绝 query/fragment；东方财富拒绝 fragment、保留既有 query 并以请求参数覆盖同名键；新浪日线保留既有 query 并覆盖自身参数，当前不显式拒绝 fragment。
- 新行情来源的日线长度、响应体、公司行动页数和页大小都有硬上限；旧 `SinaBroker` 的无上限读体属于已记录兼容边界。
- 错误与日志只保留来源、状态码、内容类型、长度和稳定分类，不记录完整响应、Cookie、Token 或 URL 凭据。
- HTTP 调用不得发生在数据库事务内。

## 7. 测试与验收证据

Fixture、受控 `RoundTripper` 与测试服务器覆盖新行情适配器的严格解析、日期/数值边界、分页完整性、错误分类、限频、重试、取消、响应上限、期货白名单和已知上游异常。限频断言记录请求进入 Transport 的时间，避免本地网络调度和首个响应耗时造成伪失败。旧兼容 Broker 按本节单独验证其既有重试与错误行为。

```bash
go test ./pkg/broker ./pkg/indicator -cover
```

## 8. 相关文档

- [系统设计](../../architecture/system-design.md)
- [行情领域设计](../internal/market.md)
- [应用层设计](../internal/application.md)
- [端口设计](../internal/port.md)
