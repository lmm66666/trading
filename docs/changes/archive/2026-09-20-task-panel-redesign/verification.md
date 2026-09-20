---
id: CHG-2026-09-20-task-panel-redesign-VERIFICATION
result: passed
authority: evidence
---

# Verification

Change lifecycle: [requirements](requirements.md). Approved target: [design](design.md).

`result` is pending, passed, failed, or blocked; it is not the change lifecycle.

## Traceability

| Requirement | Design | Implementation | Test | Result |
|---|---|---|---|---|
| REQ-TPR-001 | design §Current and target behavior | ScanPanel/BacktestPanel 双栏布局、styles.css（899px 断点） | ScanPanel/BacktestPanel 测试 + `npm run check` | passed |
| REQ-TPR-002 | design §Interface | features/strategy/RangePicker.tsx（含 portal 弹层修复） | RangePicker.test.tsx 10 用例 | passed |
| REQ-TPR-003 | design §Interface | ScanPanel/BacktestPanel/StrategyForm 控件重组 | 面板测试（chips aria-pressed、高级费用折叠、param-grid） | passed |
| REQ-TPR-004 | design §Current and target behavior | 面板结果区空态 | 面板空态用例（"尚未发起扫描/回测"） | passed |
| REQ-TPR-005 | design §Compatibility | taskUtils 复用，submit 路径未改 | 既有提交体/幂等键/轮询/分页回归全绿 | passed |

## Executed checks

- `npm --prefix web run check`（tsc + vitest + build + 覆盖率门禁）：通过。实现提交后与评审修复提交后各执行一次，均为 exit 0；web 测试 142 个（20 文件）全部通过。
- `npx vitest run src/features/strategy/RangePicker.test.tsx src/features/scan/ScanPanel.test.tsx src/features/backtest/BacktestPanel.test.tsx`：34 个用例全部通过（评审修复后复跑）。
- `go test ./...`：通过（含 `TestDocumentationContract`/`TestDocumentationLinks` 文档门禁）。
- `go vet ./...`：通过。
- `bash scripts/verify.sh`：通过（10 项本地门禁全部通过）。MySQL/镜像门禁未选择：本变更不涉及数据库语义与构建部署，记录为不适用。

## Coverage

评审阶段 `npm --prefix web run check` 输出：语句 92.31% / 分支 83.3% / 函数 89.36% / 行 94.21%，高于 80% 总门禁；评审修复提交后门禁复跑通过（脚本内含阈值检查）。工程标准中 90% 分档仅适用于 Go 核心模块（行情、指标、策略、回测），web 不适用该分档。

## Deviations and remaining risk

- 原型进度条未实现（后端 `RunStatus` 无进度字段），属批准范围内的设计偏差，已在 requirements 声明。
- design.md 于批准后做了一次机械性链接修正（`../../design/web.md` → `../../../design/web.md`，修复相对路径层级），无语义变化；`approved_revision` 对应修正前正文。
- 独立评审（子 Agent，只读）发现 1 项 major：弹层 `position: absolute` 被配置列 `overflow-y: auto` 裁剪，桌面双栏下必现横向截断。已修复（提交 933e8ce）：弹层经 `createPortal` 渲染到 `body`、按触发器 rect fixed 定位（底部溢出翻转上方）、外部点击判定区分弹层内节点、列滚动即收起、打开聚焦/关闭焦点返回触发器（非模态 dialog 语义）、预设按钮补 `aria-pressed`。
- 评审遗留非阻断项：三个测试文件各自实现 `fmt`/`today`/`monthsAgo` 辅助（组件与测试分离属测试独立性考量，测试间重复可后续收敛到共享 test-utils）；清除按钮回写空区间但不关闭弹层（原型设计意图）；选中区间恰好等于某预设时重开弹层高亮可能过期（纯外观）。
- 弹层 fixed 定位在视口 resize 时不重排（重新打开即重新计算），交互可接受。

## Evidence revision and blockers

- 源码修订：`codex/task-panel-redesign` 提交 8374de6（实现）+ 933e8ce（评审修复）；基线为 main 1e267c9。
- 独立子 Agent 评审结论：无 blocker；major 已修复，minor/note 处置见上节。
- web.md 第 1/2/5/8 节已按设计回填（同批提交）。
- 无未决阻塞。
