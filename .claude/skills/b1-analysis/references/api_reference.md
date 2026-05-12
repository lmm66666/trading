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
```

| 参数     | 说明                          |
|----------|-------------------------------|
| strategy | `daily_b1_buy` 或 `weekly_b1_buy` |

**注意**：扫描数据库耗时较长，建议超时 60s。

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

## 数据字段说明

本 skill 评分时重点关注的数据字段：

**K 线数据**：`date`, `open`, `high`, `low`, `close`, `volume`

**财报数据**：`total_revenue`, `net_profit`, `net_profit_cut`, `gross_margin`, `net_margin`, `roe`, `roa`, `asset_liability_ratio`, `operating_cash_flow`, `eps`

完整的请求/响应格式、字段类型、分页说明等，请阅读 `api/api.md`。
