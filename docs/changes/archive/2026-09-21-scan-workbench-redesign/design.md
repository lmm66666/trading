---
id: CHG-2026-09-21-scan-workbench-redesign-DESIGN
approval_status: approved
authority: proposed
approved_by: user
approved_at: "2026-09-21T17:07:35+08:00"
approved_revision: "4c2217b"
approved_scope: ["REQ-SCAN-UI-001–006", "design.md 全部章节"]
---

# 扫描工作台重设计 design

## Current and target behavior

**当前**：扫描页左栏为配置卡（策略/时间/范围），右栏为独立状态条与结果表两张卡片。状态条平铺完整 run_id；结果表仅代码与信号时间两列；0 入选无空态；「策略」标签重复。

**目标**：

- 右栏合并为单张结果卡。卡头状态条：状态徽标 + `<strategy> v<version>`；「任务详情」展开后显示 run_id、数据版本、尝试次数与取消标记；取消按钮在可取消状态下保持在卡头可见。状态条不显示扫描时间范围与完成时间：`RunStatus` 契约（`runDetails`）不含这两项，`port.Run` 无完成时间字段，用户 2026-09-21 二次裁决缩减状态条而非扩展任务契约。
- 卡体：入选计数 + 失败徽标；表格列为代码（等宽）、名称、信号时间、信号原因；行点击沿用 `onSelectInstrument` 跳图表。0 入选显示空态文案与调整建议；加载中显示骨架行；失败列表保持 `<details>` 折叠，新增名称列。
- 左栏：删除 StrategyForm 内部重复的「策略」字段 label（区块标题保留）；策略元信息（主周期、预热根数、默认持有根数）以 chip 展示；既有交易所 chip 保留（扫描范围为交易所全市场，不新增当前标的/复权 chip，避免误导）；参数保持英文参数名作为 label（用户裁决，不改内核与策略目录契约），placeholder 仍为「默认 X」。
- 回测页共用的 RunMonitor、StrategyForm 同步获得精简状态条与去重复标签，回测页布局本身不变。

已确认的扫描语义（设计前提，本变更不改变）：一次扫描每只股票最多一个信号行（`ReplayLatest` 只取窗口最后一根 bar 的决策，`SignalSnapshot.Validate` 强制 instrument 唯一）；各行信号时间通常为窗口最后一根 bar 收盘时间，停牌股可能更早，故信号时间列保留。

## Data sources, model, and flow

**证券名称**：

```text
t_signal_snapshot_rows（instrument_id） 读取分页
  → 存储层按本页 instrument_id 集合关联 t_instruments 取 name
  → port.SnapshotRow / Failure 的 DTO 投影新增 name（可选）
  → 前端表格名称列，缺失回退代码
```

- 名称解析放在读取路径（MySQL 存储层分页查询 join 或按页批量查询），一次分页最多 100 行，等量级一次小批量名称查询；不改快照写入、不改 `t_signal_snapshot_rows` 表结构。
- 名称为读取时点 `t_instruments` 当前值，属显示属性；证券不在 `t_instruments` 或名称为空时 DTO 省略 `name` 字段，前端回退显示代码。
- `failures` 行同样附加 `name`（同一批 instrument_id 一次解析，无额外查询）。

## Interface and contract changes

纯新增可选字段，无字段语义变化：

1. `GET /api/v1/signal-snapshots/latest`：`rows[]` 与 `failures[]` 元素新增 `name?: string`；无名称时字段省略。
2. HTTP 契约文档（`docs/standards/http-api.md`）同步该端点的响应字段表与示例，并写明「名称为读取时点显示属性，不随快照固化」。

`/api/v1/strategies` 契约不变。

## Errors, degradation, and recovery

- 名称查询失败：读取路径不因名称缺失而失败；降级为省略 `name`（与「证券已删除」同一处理），错误仅记录日志，不影响快照分页主体。
- 前端旧缓存/旧客户端：忽略新字段，行为不变。

## Concurrency, capacity, performance, and security

- 无新并发模型；名称解析在既有分页读取调用内完成，单页 ≤100 行的批量查询不引入 N+1。
- 无新安全面：名称为既有搜索接口已公开的数据。

## Compatibility, migration, and rollback

- 无持久化变更、无迁移；API 向后兼容（只增可选字段）。
- 回滚：还原代码即可，无数据残留；契约文档随代码版本回滚。

## Alternatives and tradeoffs

- **前端按行逐个查询证券名称**（已否决）：结果页最多 100 行产生 N+1 请求，且搜索接口语义与名称解析不同。
- **快照写入时固化名称**（已否决）：名称为显示属性，固化会使证券更名后旧快照显示过期名称，且需要迁移历史行。
- **策略参数中文 label 由服务端下发**（已否决）：用户裁决参数直接显示英文即可，本次不改 `/api/v1/strategies` 与内核参数元数据。
- **扩展 RunStatus 契约投影完成时间与扫描时间范围**（已否决）：`port.Run` 无完成时间字段，需 `t_compute_runs` 加列与迁移；用户 2026-09-21 二次裁决缩减状态条信息（徽标 + 策略版本），不扩展任务契约。

## Test strategy

- Go：快照读取名称关联的应用层/存储层测试（存在名称、缺失名称、failures 行名称）；契约测试覆盖端点新字段。
- 前端：RunMonitor 精简渲染与详情展开、ScanResults 四列表格/空态/骨架/名称回退、StrategyForm 去重复标签、ScanPanel 布局回归；既有测试同步更新。
- 门禁：`npm --prefix web run check`、`go test ./...`、`go vet ./...`、`bash scripts/verify.sh`；涉及 MySQL 读取路径变更，按工程标准风险触发条件追加 `--mysql` 做远端随机隔离库验收。

## Affected current designs, standards, and ADRs

- `docs/standards/http-api.md`：快照端点响应契约。
- `docs/design/internal/infrastructure/mysql.md`：快照分页读取关联 `t_instruments` 名称。
- `docs/design/internal/application.md`：快照读取投影包含显示用名称（如该文档已约定读取投影内容，否则仅关联引用）。
- `docs/design/workflows/strategy-scan.md`：结果行展示语义说明（名称为读取时点显示属性；每股最多一行的既有语义引用）。
- 前端暂无独立设计文档所有者；组件行为以前述契约与本设计为准，不新增 owns 声明。

## Approval scope and evidence

用户于 2026-09-21T17:07:35+08:00 批准，批准内容版本 `4c2217b`，范围为 REQ-SCAN-UI-001–006 与 design.md 全部章节。本次批准取代首次批准（2026-09-21T16:53:22+08:00，版本 `4f9789a`），差异为 REQ-001 状态条缩减为契约已有字段。
