# Database Cutover And Branch Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 安装本机 MySQL CLI，将 `trading` 的旧行情完整迁移到版本化策略内核，验证后可恢复地清理废弃表，并把已验证代码合入 `main`。

**Architecture:** 使用仓库内的幂等迁移器完成 dry-run、分批写入和摘要校验；用 MySQL CLI 负责迁移前盘点、逻辑备份、上线属性补齐、迁移后独立核对及旧表删除。数据库变更完成后执行项目完整门禁和独立代码审查；当前开发分支由本地 `main` 直接创建，因此不存在额外中间开发分支，最终以 fast-forward 合入本地 `main`。

**Tech Stack:** Go 1.25.7、GORM、MySQL 5.7/8.0、Homebrew `mysql-client`、Git。

**Spec:** `docs/superpowers/specs/2026-09-13-strategy-backtest-kernel-design.md`

## Global Constraints

- 不在命令行参数、日志或计划文档中暴露 MySQL 密码；CLI 通过交互式 `-p` 输入密码。
- 删除任何旧表前必须完成带结构和数据的逻辑备份，并记录备份路径与 SHA-256。
- 只有迁移报告为 `COMPLETE`、`BacktestEnabled=true`、无拒绝代码和失败证券，且独立 SQL 计数核对通过后才能清理旧表。
- 保留仍由运行时财报/股票信息业务使用的 `t_stock_info`；仅清理经代码引用和数据库验证共同确认废弃的行情源表与迁移检查点表。
- 不覆盖 `.claude/worktrees/`、`.tmp/` 或其他用户未提交内容。
- 合并前执行所有测试、覆盖率门禁、Race Detector、`go vet`、性能测试及可用环境下的 MySQL/Docker 验证。

---

### Task 1: 工具链与代码基线

**Files:**
- Modify: `docs/superpowers/plans/2026-09-13-database-cutover-and-branch-integration.md`

**Interfaces:**
- Consumes: 当前 `codex/backtest-kernel-design` 工作树和 Homebrew。
- Produces: 可调用的 `mysql`/`mysqldump`，以及通过快速测试的迁移代码基线。

- [ ] **Step 1: 安装并定位 MySQL CLI**

运行 `brew install mysql-client`；使用 `brew --prefix mysql-client` 得到 keg-only 路径，并直接调用其中的 `bin/mysql`、`bin/mysqldump`，不修改用户 shell 配置。

- [ ] **Step 2: 验证客户端与服务端连接**

运行 `<mysql-prefix>/bin/mysql --host=192.168.31.85 --port=45709 --user=root -p --database=trading --connect-timeout=10 --execute='SELECT VERSION(), DATABASE(), CURRENT_USER(), 1'`，预期认证成功且数据库为 `trading`。

- [ ] **Step 3: 验证迁移代码基线**

运行 `go test ./... && go vet ./...`，预期全部退出码为 0；若失败，停止数据库写入并先定位根因。

- [ ] **Step 4: 提交计划文档**

运行 `git add docs/superpowers/plans/2026-09-13-database-cutover-and-branch-integration.md && git commit -m "docs: plan database migration cutover"`。

### Task 2: 数据库盘点与可恢复备份

**Files:**
- Create: `/tmp/trading-precleanup-<random>.sql`
- Create: `/tmp/trading-precleanup-<random>.sql.sha256`

**Interfaces:**
- Consumes: 可用 MySQL CLI 和只读数据库权限。
- Produces: 迁移前表结构/行数快照，以及可恢复的全库逻辑备份。

- [ ] **Step 1: 盘点表、版本与源数据规模**

通过 `information_schema.tables` 查询 `trading` 全部表及行数估算，并精确执行旧表 `COUNT(*)`；若新内核表存在，同时查询 `t_market_data_versions`、`t_instruments` 和 `t_market_bars` 的状态与计数。

- [ ] **Step 2: 创建唯一备份文件**

用 `mktemp /tmp/trading-precleanup-XXXXXX.sql` 分配路径，再执行 `mysqldump --single-transaction --quick --routines --triggers --events --default-character-set=utf8mb4` 写入该路径。预期退出码为 0 且文件非空。

- [ ] **Step 3: 校验备份完整性**

运行 `shasum -a 256 <backup>` 写入同路径 `.sha256`，并确认备份包含旧行情表及其 `CREATE TABLE`/`INSERT` 内容；不得打印密码。

### Task 3: 幂等迁移与独立校验

**Files:**
- Create then delete: `/tmp/codex-trading-schema-init/main.go`
- Read: `cmd/migrate-strategy-kernel/main.go`
- Read: `internal/infrastructure/mysql/legacy_migrator.go`
- Read: `internal/infrastructure/mysql/legacy_migration_target.go`

**Interfaces:**
- Consumes: `t_stock_info`、`t_stock_kline_daily`、`t_stock_kline_weekly` 和东方财富行情源。
- Produces: `COMPLETE` 的版本 1，以及经过摘要验证的 `t_instruments`、`t_market_bars`、复权因子和公司行为。

- [ ] **Step 1: 在空目标库初始化新内核 schema**

若盘点确认新内核表均不存在，使用一次性 Go helper 解析 `config.yaml` 并仅调用 `data.New(wrapper.Config.DB)` 后立即关闭连接。该调用只执行项目现有 `runtimeModels` 和 `mysql.Migrate`，不启动 HTTP、Worker 或调度器；运行后删除 helper，并用 `information_schema` 验证所有迁移模型和索引存在。

- [ ] **Step 2: 执行只读 dry-run**

运行 `go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=true -batch-size=1000`。预期报告无 `RejectedCodes`、无 `Failures`、`Quality=COMPLETE`。

- [ ] **Step 3: 执行可重入写迁移**

运行 `go run ./cmd/migrate-strategy-kernel -config config.yaml -dry-run=false -batch-size=1000`。进程中断时只重跑同一命令，依靠检查点续传，不手工修改中间状态。

- [ ] **Step 4: 校验迁移报告与数据库状态**

确认报告为 `Version=1`、`Quality=COMPLETE`、`BacktestEnabled=true`、失败/拒绝列表为空；SQL 独立确认版本 1 已发布，证券数和日/周 Bar 数与报告一致，且不存在孤儿 `instrument_id` 或非正价格。

- [ ] **Step 5: 补齐上线属性**

仅对来源为 `legacy-strategy-kernel-v2` 且迁移验证通过的证券设置 `active=1`、`lot_size=100`，并按交易所与代码前缀填充 `board`（北交所 `BSE`、科创板 `STAR`、创业板 `CHINEXT`、其余 `MAIN`）。随后确认全部迁移证券均为 active、lot size 为正且 board 非空。

### Task 4: 清理废弃表并验证运行时

**Files:**
- Delete from database after verification: `t_stock_kline_daily`
- Delete from database after verification: `t_stock_kline_weekly`
- Delete from database after verification: `t_legacy_kernel_migration`
- Preserve in database: `t_stock_info`

**Interfaces:**
- Consumes: 已验证的新内核数据和可恢复逻辑备份。
- Produces: 不含旧行情冗余表的运行数据库。

- [ ] **Step 1: 最后确认生产路径引用**

运行 `rg` 确认生产启动、行情查询、扫描和回测均不访问两个旧 K 线表；确认 `t_stock_info` 仍被 `business.StockInfoProvider` 使用，不能删除。

- [ ] **Step 2: 原子删除已确认废弃表**

在明确数据库 `trading` 后执行带反引号限定的 `DROP TABLE IF EXISTS t_stock_kline_daily, t_stock_kline_weekly, t_legacy_kernel_migration`。删除前再次确认备份文件非空且 SHA-256 可复算。

- [ ] **Step 3: 验证清理结果与核心查询**

查询 `information_schema` 确认三个表不存在、保留表仍存在、版本 1 仍为 `COMPLETE`；执行版本化行情批量读取、策略扫描/回测相关测试，确认不依赖已删除表。

### Task 5: 完整门禁、代码审查与分支集成

**Files:**
- Read: `scripts/verify.sh`
- Modify if review finds defects: exact affected implementation and matching test files only

**Interfaces:**
- Consumes: 已迁移数据库与当前开发分支。
- Produces: 通过门禁和独立审查、合入本地 `main` 的代码。

- [ ] **Step 1: 执行完整验证**

运行 `bash scripts/verify.sh`。预期测试、覆盖率、Race Detector、`go vet`、5000 证券性能、MySQL 5.7/8.0 集成测试和 Docker 镜像检查全部通过；环境性跳过或失败必须如实记录，不能当作通过。

- [ ] **Step 2: 启动独立代码审查**

启动子 agent 审查 `main..codex/backtest-kernel-design` 的正确性、回归风险、无用代码和数据库迁移安全性；对有效问题按测试驱动方式修复并重新运行相关门禁。

- [ ] **Step 3: 同步远程引用并确认拓扑**

运行 `git fetch github`，确认当前分支由本地 `main` 直接创建、工作树只含用户既有未跟踪目录、`main` 未出现未知的新提交。若远程 `main` 出现本地尚未包含的提交，先在开发分支合并并重新验证。

- [ ] **Step 4: 合入并清理开发分支**

当前没有独立的中间“原分支”：`codex/backtest-kernel-design` 的创建点就是本地 `main`。切换 `main` 后执行 `git merge --ff-only codex/backtest-kernel-design`，再次运行快速验证，再删除已合并分支 `git branch -d codex/backtest-kernel-design`；不处理用户的 `worktree-remove-scorer` 工作树/分支，也不在未授权情况下 push。
