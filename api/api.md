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

### 1. 保存股票历史数据

从行情数据源（Broker）获取指定股票的历史 K 线数据，清洗后写入数据库。

- **Method**: `POST`
- **Path**: `/api/stocks/historical`
- **Content-Type**: `application/json`

#### 请求参数

| 字段 | 类型   | 必填 | 说明                     |
|------|--------|------|--------------------------|
| code | string | 是   | 股票代码，如 `600312`    |

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
  "data": null
}
```

---

### 2. 补全股票数据

手动触发扫描，检查 daily 和 weekly 表中所有股票代码的数据完整性，自动补充缺失的日线和周线数据。

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
  "data": null
}
```

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

### 4. 股票买点扫描

按指定策略名称扫描所有股票，判断最新数据日期是否为买点，返回符合条件的股票代码列表。

- **Method**: `GET`
- **Path**: `/api/stocks/signal`
- **说明**: 需要扫描数据库，耗时较长。日线/周线 B1 建议超时 30s；`bottom_surge_pullback` 策略涉及全量并发扫描，建议超时 60s

#### 请求参数

| 字段     | 类型   | 必填 | 说明                                 |
|----------|--------|------|--------------------------------------|
| strategy | string | 是   | 策略名称，如 `daily_b1_buy`          |

#### 请求示例

```bash
# 日线 B1 策略
curl "http://localhost:8080/api/stocks/signal?strategy=daily_b1_buy"

# 周线 B1 策略
curl "http://localhost:8080/api/stocks/signal?strategy=weekly_b1_buy"

# 底部倍量回调策略
curl "http://localhost:8080/api/stocks/signal?strategy=bottom_surge_pullback"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "strategy": "daily_b1_buy",
    "codes": ["600312", "000001"]
  }
}
```

#### 响应字段说明

| 字段     | 类型     | 说明                   |
|----------|----------|------------------------|
| strategy | string   | 策略名称               |
| codes    | []string | 符合该策略的股票代码   |

#### 支持的策略名称

| 策略名称                 | 说明                                                         |
|--------------------------|--------------------------------------------------------------|
| `daily_b1_buy`          | 日线 B1：倍量拉升（量比≥2.0，涨幅≥5%）+ 缩量回调 + KDJ低位（<40）+ MA20向上 |
| `weekly_b1_buy`         | 周线 B1：KDJ超卖（<10）+ MA20向上                                        |
| `bottom_surge_pullback` | 底部倍量回调：底部确认（放量日Open在60日低点上浮15%内）+ 倍量拉升（允许间隔3天）+ 缩量回调（量能回归VMA20*1.5内）+ 不破MA20 + KDJ低位（5~40） |

---

### 5. 策略回测

对单只股票进行策略回测，返回历史上所有满足该策略的买入信号日期。

- **Method**: `GET`
- **Path**: `/api/stocks/backtest`

#### 请求参数

| 字段     | 类型   | 必填 | 默认值   | 说明                                 |
|----------|--------|------|----------|--------------------------------------|
| code     | string | 是   | -        | 股票代码，如 `600150`               |
| strategy | string | 是   | -        | 策略名称，同买点扫描接口             |
| cycle    | string | 否   | 策略默认 | 周期：`daily`（日线）或 `weekly`（周线）|

#### 请求示例

```bash
# 回测中国船舶的底部倍量回调策略
curl "http://localhost:8080/api/stocks/backtest?code=600150&strategy=bottom_surge_pullback"

# 回测日线 B1 策略
curl "http://localhost:8080/api/stocks/backtest?code=600150&strategy=daily_b1_buy"

# 回测周线 B1 策略（显式指定周期）
curl "http://localhost:8080/api/stocks/backtest?code=600150&strategy=weekly_b1_buy&cycle=weekly"
```

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "code": "600150",
    "strategy": "bottom_surge_pullback",
    "cycle": "daily",
    "signals": [
      { "date": "2026-04-29" },
      { "date": "2026-05-14" },
      { "date": "2026-05-15" }
    ]
  }
}
```

#### 响应字段说明

| 字段     | 类型     | 说明                   |
|----------|----------|------------------------|
| code     | string   | 股票代码               |
| strategy | string   | 策略名称               |
| cycle    | string   | 数据周期               |
| signals  | []object | 历史买入信号列表       |

**signals 数组元素字段：**

| 字段 | 类型   | 说明           |
|------|--------|----------------|
| date | string | 信号日期，格式 YYYY-MM-DD |

---

### 5. 查询股价 K 线数据

根据股票代码和周期查询 K 线数据，支持分页。

- **Method**: `GET`
- **Path**: `/api/stocks/price`

#### 请求参数

| 字段     | 类型   | 必填 | 默认值   | 说明                                 |
|----------|--------|------|----------|--------------------------------------|
| code     | string | 是   | -        | 股票代码，如 `600312`               |
| cycle    | string | 否   | `daily`  | 周期：`daily`（日线）或 `weekly`（周线）|
| pagesize | int    | 否   | `20`     | 每页条数                             |
| pagenum  | int    | 否   | `1`      | 页码，从 1 开始                      |

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
    "data": [
      {
        "code": "600312",
        "date": "2022-01-22",
        "open": 10.5000,
        "high": 11.2000,
        "low": 10.3000,
        "close": 10.8000,
        "volume": 1234567
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
| data     | []object | K 线数据列表           |

**data 数组元素字段：**

| 字段   | 类型    | 说明           |
|--------|---------|----------------|
| code   | string  | 股票代码       |
| date   | string  | 日期，格式 YYYY-MM-DD |
| open   | float64 | 开盘价         |
| high   | float64 | 最高价         |
| low    | float64 | 最低价         |
| close  | float64 | 收盘价         |
| volume | int64   | 成交量（股）   |

---

### 6. 补全财报数据

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

### 7. 查询财报数据

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

### 8. 财报信号扫描

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

### 9. 查询 Shibor 利率

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

### 10. 查询汇率

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