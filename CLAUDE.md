# 项目概述
一个基于 Go 的股票数据收集与分析平台，从新浪财经获取 A 股行情与财报数据，提供技术指标计算、策略扫描和 HTTP API 查询能力，辅助投资决策。

## 核心功能
- **行情数据**：日线/周线历史 K 线拉取与存储
- **财报数据**：季度财报核心指标（利润表、盈利能力、偿债能力、运营效率、现金流）
- **增量更新**：对比数据库已有数据，只补充缺失部分
- **策略扫描**：基于 MA、MACD、KDJ、成交量等指标的全市场买点扫描，以及基于财报数据的净利润连续增长筛选
- **HTTP API**：数据写入、数据查询、策略扫描的统一 RESTful 接口
- **批量脚本**：Shell 脚本批量拉取多只股票数据

# 项目结构

```
trading/
├── go.mod / go.sum          # Go 模块（go 1.25.7，依赖 gin、gorm、x/text）
├── main.go                  # 程序入口（初始化各层并启动 gin server）
├── README.md                # 项目说明与快速开始指南
├── config.yaml              # 应用配置文件（DB 连接等）
├── config.example.yaml      # 配置模板（复制后修改使用）
├── .gitignore               # Git 忽略规则
├── Dockerfile               # 容器镜像构建（多阶段，固定 linux/amd64）
├── .dockerignore            # Docker 构建上下文忽略规则
├── config/
│   └── config.go            # 配置结构体定义与加载
├── model/                   # 数据模型层
│   ├── README.md            # Model 规范（进入该目录时必须优先读取）
│   ├── stock_kline.go       # 通用 K 线数据模型（GORM）
│   ├── stock_kline_daily.go # 日线数据模型（表 t_stock_kline_daily）
│   ├── stock_kline_weekly.go # 周线数据模型（表 t_stock_kline_weekly）
│   └── financial_report.go  # 财报数据模型（表 t_financial_reports）
├── data/                    # 数据访问层（Repository）
│   ├── data.go              # Data 入口，管理数据库连接与各模型 Repo
│   ├── stock_kline_daily.go # 日线数据仓库接口与实现
│   ├── stock_kline_weekly.go # 周线数据仓库接口与实现
│   └── financial_report.go  # 财报数据仓库接口与实现
├── business/                       # 业务逻辑层
│   ├── stock_service.go           # StockDataService：行情数据拉取、保存
│   ├── stock_service_test.go
│   ├── financial_service.go       # FinancialReportService：财报数据拉取、保存
│   ├── financial_service_test.go
│   ├── signal_service.go          # SignalService：策略扫描（买点信号）
│   ├── signal_service_test.go
│   ├── query_service.go           # QueryService：股价/财报数据查询
│   ├── query_service_test.go
│   ├── scheduler_base.go          # 通用调度器基础设施（triggerGuard、concurrentWorker）
│   ├── scheduler_base_test.go
│   ├── stock_scheduler.go         # 行情增量调度器
│   ├── stock_scheduler_test.go
│   ├── financial_scheduler.go     # 财报增量调度器
│   ├── util.go                    # 工具函数（toSymbol、cleanKlines 等）
│   └── util_test.go
├── api/                     # HTTP API 层（gin，一个接口一个文件）
│   ├── api.md               # API 接口文档（含 curl 示例）
│   ├── README.md            # API 规范
│   ├── router.go            # gin 路由注册
│   ├── handler.go           # 公共 handler 结构体与响应方法
│   ├── handler_test.go      # 公共 mock 与测试工具
│   ├── save_stock_historical_data.go    # POST /api/stocks/historical
│   ├── save_stock_historical_data_test.go
│   ├── append_stock_data.go             # POST /api/stocks/append
│   ├── append_stock_data_test.go
│   ├── save_financial_report_data.go    # POST /api/stocks/financial-report
│   ├── save_financial_report_data_test.go
│   ├── append_financial_report_data.go  # POST /api/stocks/financial-report/append
│   ├── append_financial_report_data_test.go
│   ├── get_stock_buy_signals.go         # GET /api/stocks/signal
│   ├── get_stock_price.go               # GET /api/stocks/price
│   ├── get_stock_price_test.go
│   ├── get_financial_report.go          # GET /api/stocks/financial-report
│   ├── get_financial_report_test.go
│   ├── get_financial_report_signal.go   # GET /api/stocks/financial-report/signal
│   └── get_financial_report_signal_test.go
├── pkg/
│   ├── broker/              # 行情数据提供者
│   │   ├── broker.go        # IBroker 统一接口
│   │   ├── sina.go          # SinaBroker（新浪财经实现）
│   │   └── sina_test.go     # 接口测试
│   ├── indicator/           # 技术指标计算工具（纯计算，无业务逻辑）
│   │   ├── round.go         # Round4 四舍五入工具
│   │   ├── ma.go            # SMA / EMA / 成交量均线
│   │   ├── macd.go          # MACD 计算
│   │   ├── macd_test.go
│   │   ├── kdj.go           # KDJ 计算
│   │   ├── kdj_test.go
│   │   ├── limiter.go
│   │   └── limiter_test.go
│   ├── filter/              # 过滤器层
│   │   ├── filter.go        # IFilter 接口、Result 定义（K 线）
│   │   ├── filter_test.go
│   │   ├── kdj.go           # KDJ 超买/超卖过滤器
│   │   ├── kdj_test.go
│   │   ├── ma.go            # MA 趋势过滤器
│   │   ├── ma_test.go
│   │   ├── volume_surge.go  # 放量上涨后回调过滤器
│   │   ├── volume_surge_test.go
│   │   ├── date.go          # 持有天数过滤器
│   │   ├── date_test.go
│   │   └── financial/       # 财报过滤器
│   │       ├── filter.go    # IFinancialFilter 接口、通用 computeGrowthFilter
│   │       ├── profit_growth.go   # 净利润同比增长过滤器
│   │       ├── profit_growth_test.go
│   │       ├── revenue_growth.go  # 营收同比增长过滤器
│   │       └── revenue_growth_test.go
│   └── strategy/            # 策略层（组合多个 filter）
│       ├── strategy.go      # Strategy 结构体、Signal、Scan / ScanAll（K 线）
│       ├── strategy_test.go
│       ├── financial.go     # FinancialStrategy（财报 filter 组合）
│       ├── financial_test.go
│       ├── buy.go           # 预定义买入策略（如 B1）
│       └── sell.go          # 预定义卖出策略
├── .claude/
│   ├── settings.local.json      # 本地 IDE 设置
│   └── skills/                  # Claude Code 自定义 skill
│       └── stock/               # 股票分析 skill（含底部倍量、宏观流动性两种模式）
├── shell/                   # 脚本工具
    ├── save_stock_historical.sh   # 批量保存股票历史 K 线数据
    ├── save_financial_report.sh   # 批量保存股票财报数据
    └── code/                      # 股票代码列表
        └── 上海.txt
```

# 依赖与启动

## 环境要求
- Go 1.25.7+
- MySQL 5.7+ 或 8.0+

## 快速启动
1. 创建数据库：`CREATE DATABASE trading CHARACTER SET utf8mb4;`
2. 复制配置文件：`cp config.example.yaml config.yaml` 并修改数据库连接信息
3. 安装依赖：`go mod download`
4. 启动服务：`go run .`（默认读取 `config.yaml`，也可通过 `-config` 指定路径）

服务默认监听 `:8080`，启动后会自动执行 `AutoMigrate` 创建数据表。

## Docker 启动

**1. 构建镜像**

```bash
# 需显式指定平台(例如使用 buildx)
docker buildx build --platform linux/amd64 -t trading:latest --load .
```

**2. 运行容器**

```bash
docker run -d --name trading -p 8080:8080 trading:latest
```

**3. 导出镜像**

```bash
docker save -o trading.tar trading:latest  
```

# 开发规范
## 强制要求
- 本项目除本文档外，还遵循全局 `~/.claude/CLAUDE.md`
- 在读取或分析任何目录下的代码时，**如果该目录下存在 README.md，必须先读取 README.md**，以了解该目录的规范、约束和上下文，避免误读代码
- 复杂需求完成后，启动子 agent 调用 /simplify 复查可优化点，并调用 /code-review 审查代码
- 需求交付前，检查并同步更新 CLAUDE.md、README.md 及相关注释

## 代码风格
- 遵循 Go 标准编码规范

## 测试要求
- 新增业务逻辑必须配套单元测试
- 测试覆盖率保持 80% 以上（继承全局规范） 

