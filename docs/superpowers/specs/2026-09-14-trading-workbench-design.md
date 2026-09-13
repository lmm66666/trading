# A 股图表工作台设计

## 目标

在现有 Go 行情与指标内核之上提供一个桌面优先、移动端可用的单页图表工作台。第一页只包含证券搜索、日/周 K 线、成交量、MA5/MA20/MA60，以及可添加到主图或独立窗格的技术指标。

## 范围

第一期支持：

- 按六位代码前缀或中文名称搜索活跃 A 股。
- 使用完整证券身份 `SSE:600000`、`SZSE:000001`、`BSE:920001`。
- 日线与周线，原始价与前复权价。
- K 线、成交量、SMA、EMA、MACD、KDJ。
- 固定行情版本的时间游标加载，图表向左滚动时加载更早数据。
- 桌面两列布局；窄屏将搜索栏变为抽屉。
- 暗色主题、A 股红涨绿跌、键盘搜索、加载/错误/空状态。

第一期不支持实时分时、交易、画线、自选股云同步、预警、新闻、公司详情、拼音搜索、用户账户或脚本指标。

## 架构

前端放在 `web/`，使用 React、TypeScript、Vite 和 Lightweight Charts。Go 服务新增证券目录查询与图表聚合查询；生产镜像在 Node 构建阶段生成静态文件，再由现有 Go 服务同源托管。开发时 Vite 将 `/api` 代理到 `:8080`，不增加宽松 CORS。

证券搜索经 `api -> application -> port -> infrastructure/mysql`。图表查询经 `api -> application -> market/indicator + port.MarketData`，先锁定大于零的 COMPLETE 行情版本，再读取最多二十年历史作为稳定计算上下文，最后裁剪到请求窗口。任何一次翻页都携带首次响应的 `data_version`，禁止混合版本。

## API

### 证券搜索

`GET /api/v1/instruments?q=<query>&exchange=<optional>&limit=<1..50>`

`q` 为 1–128 字节。纯数字按代码前缀匹配；其他输入按名称包含匹配。只返回活跃证券。排序优先级为精确代码、代码前缀、名称、交易所、代码。

每项包含 `instrument`、`code`、`name`、`exchange`、`board`、`lot_size`。无匹配返回空数组，不是 404。

### 图表查询

`POST /api/v1/chart-queries`

请求包含：

- `instrument`：完整证券身份。
- `timeframe`：`DAY` 或 `WEEK`。
- `price_view`：`RAW` 或 `QFQ`。
- `before`：可选 RFC3339 排他游标。
- `limit`：100–1000，默认 400。
- `data_version`：0 表示解析最新 COMPLETE，正数表示固定版本。
- `indicators`：最多 16 项，支持 SMA、EMA、MACD、KDJ 的有界参数。

响应包含证券摘要、`data_version`、升序 bars、按时间点表示的 series、`has_more` 与 `next_before`。SMA/EMA 各返回一个 `value` 分量；MACD 返回 `dif/dea/histogram`；KDJ 返回 `k/d/j`。预热期无效点不输出，不能伪造为零。

图表服务对请求结束时间之前的完整可用历史执行指标计算，再只返回最后 `limit` 根，保证连续向前分页时重叠时间点的指标值稳定。`before` 排除同一时刻的 Bar。

## 前端体验

桌面宽度不小于 1024px 时，左栏 300px，右侧工作区占剩余宽度。顶部工具栏展示证券名称与代码、日/周、前复权/原始价、指标入口和复位缩放。主窗格默认展示 K 线与 MA5/MA20/MA60，成交量使用独立紧凑窗格；MACD/KDJ 创建独立窗格。

搜索输入防抖 200ms，支持方向键、Enter 与 Escape。当前证券、周期和复权方式写入 URL；指标配置写入 localStorage。首次无证券参数时选择搜索结果中的第一只证券；没有任何活跃证券时展示明确空状态。

暗色语义色通过 CSS 变量定义。价格数字使用 tabular figures。涨跌除红绿之外还通过符号和文字表示。图表区域提供文本摘要，并可打开最近 Bar 数据表，满足键盘和辅助技术读取需求。

## 错误与性能

搜索、图表骨架在请求超过 300ms 时可见。404 表示证券或固定版本不存在；400 表示参数非法；500 只返回脱敏错误。旧 `/api/stocks/price` 保持不变。

首屏请求 400 根 Bar，最多 16 个指标。前端不一次渲染超过 1000 根新数据；向左接近可见范围边界时才请求下一页。相同查询由前端请求缓存去重。

## 验收与测试

- Go 单元、handler、repository 和 MySQL 5.7/8.0 集成测试覆盖搜索排序、边界、版本固定、排他游标、指标分量与预热语义。
- 前端组件与状态测试覆盖搜索键盘流、URL 状态、指标开关、加载/错误/空状态和历史合并去重。
- Playwright 覆盖搜索证券、渲染默认图表、切换周期/复权、添加独立指标和加载更早 Bar。
- 总覆盖率保持 80% 以上；核心 Go 领域包保持 90% 以上。
- `go test ./... && go vet ./...`、前端 test/build、`bash scripts/verify.sh` 全部通过；需要 Docker 的步骤若环境不可用必须明确报告。

