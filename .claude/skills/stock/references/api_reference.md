# API 参考

**所有 API 接口的完整文档以项目中的 `api/api.md` 为准。**

以下仅列出本 skill 涉及的核心接口和关键参数，方便快速查阅。

---

## 核心接口速查

### 1. 股票买点扫描

```bash
# 日线 B1
curl "http://192.168.31.85:41027/api/stocks/signal?strategy=daily_b1_buy"

# 周线 B1
curl "http://192.168.31.85:41027/api/stocks/signal?strategy=weekly_b1_buy"

# 底部倍量回调
curl "http://192.168.31.85:41027/api/stocks/signal?strategy=bottom_surge_pullback"
```

| 参数     | 说明                          |
|----------|-------------------------------|
| strategy | `daily_b1_buy` 或 `weekly_b1_buy` |

**注意**：扫描数据库耗时较长，建议超时 60s（`bottom_surge_pullback` 并发扫描亦同）。

---

### 2. 查询股价 K 线数据

```bash
curl "http://192.168.31.85:41027/api/stocks/price?code=<股票代码>&cycle=<周期>&pagesize=<条数>"
```

| 参数     | 说明                          |
|----------|-------------------------------|
| code     | 股票代码，如 `600312`        |
| cycle    | `daily`（日线）或 `weekly`（周线） |
| pagesize | 每页条数，日线建议 `60`，周线建议 `20` |

---

### 3. 查询财报数据

```bash
curl "http://192.168.31.85:41027/api/stocks/financial-report?code=<股票代码>&pagesize=20"
```

| 参数     | 说明                   |
|----------|------------------------|
| code     | 股票代码               |
| pagesize | 每页条数，建议 `20`    |

---

---

## 宏观数据接口

### 4. 查询汇率

```bash
# 在岸人民币
curl "http://192.168.31.85:41027/api/macro/exchange-rate?code=USDCNY"

# 离岸人民币
curl "http://192.168.31.85:41027/api/macro/exchange-rate?code=USDCNH"

# 美元指数
curl "http://192.168.31.85:41027/api/macro/exchange-rate?code=DINIW"
```

| 参数 | 说明 |
|------|------|
| code | `USDCNY`（在岸） / `USDCNH`（离岸） / `DINIW`（美元指数） |

**响应字段**：`open`, `now`, `change_percent`

---

### 5. 查询 Shibor 利率

```bash
# 隔夜 Shibor（可作为 DR007 的近似参考）
curl "http://192.168.31.85:41027/api/macro/shibor?period=001"

# 全部期限
curl "http://192.168.31.85:41027/api/macro/shibor"
```

| 参数   | 说明 |
|--------|------|
| period | `001`=隔夜, `002`=1周, `003`=2周, `004`=1月, `005`=3月, `006`=6月, `007`=9月, `008`=1年 |

**响应字段**：`report_date`, `ir_rate`, `change_rate`

---

## 待补充接口（P0）

以下接口当前**尚未实现**，执行宏观流动性分析时需通过外部数据源（Wind / 中国货币网 / 东方财富等）补充：

| 接口 | 用途 | 推荐外部数据源 |
|------|------|----------------|
| `/api/macro/dr007` | 银行间质押式回购利率（7天），资金面核心指标 | 中国货币网 / Wind |
| `/api/macro/bond-yield?code=CN10Y` | 中国 10Y 国债收益率 | 中国债券信息网 / Wind |
| `/api/macro/bond-yield?code=US10Y` | 美国 10Y 国债收益率 | FRED / Wind |
| `/api/macro/northbound` | 北向资金近 5 日 / 20 日累计净流入 | 港交所披露易 / 东方财富 |
| `/api/macro/policy` | 近 6 个月降准降息记录、7 天逆回购利率、社融增速 | 央行官网 / Wind |

---

## 数据字段说明

本 skill 评分时重点关注的数据字段：

**K 线数据**：`date`, `open`, `high`, `low`, `close`, `volume`

**财报数据**：`total_revenue`, `net_profit`, `net_profit_cut`, `gross_margin`, `net_margin`, `roe`, `roa`, `asset_liability_ratio`, `operating_cash_flow`, `eps`

**汇率数据**：`now`（最新价）, `change_percent`（涨跌幅）

**Shibor 数据**：`ir_rate`（利率值）, `change_rate`（变化点数）

完整的请求/响应格式、字段类型、分页说明等，请阅读 `api/api.md`。
