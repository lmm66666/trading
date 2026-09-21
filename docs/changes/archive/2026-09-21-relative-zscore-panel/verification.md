---
id: CHG-2026-09-21-RELATIVE-ZSCORE-VERIFICATION
result: passed
authority: evidence
---

# 验证记录

用户批准[需求](requirements.md)及[设计](design.md)后实施。原参考Pine/Python只读；没有引入海外汇率、结算价或回溯调整伪数据。

## 需求到证据

- REQ-RZ-001：`internal/indicator/relative_panel_test.go` 验证双价格比值、同比例变化无Z、总体口径、EMA播种、长窗口、63期表现、5期收益相关、无未来前缀、非法输入。先运行缺失函数的失败测试后实现。错误的RETZ领域代码与引用已删除。
- REQ-RZ-002/003：`internal/application/chart_zscore_test.go` 验证日期as-of、商品Bar滞后、RAW/QFQ、两腿同版本且只读一次、分页相同结果、无商品/周线/读失败/版本错的隔离、取消传播、参数边界及状态。`api/query_chart_test.go` 验证comparison/lag和诊断传输。
- REQ-RZ-004：FinancialChart、zScoreBands与ZScoreSummary测试覆盖三分量分区、阈值着色、五条线、最小±2自动范围、背景随坐标缩放、诊断/缺失值/错误提示。未使用浏览器；真实浏览器视觉验收未执行。
- REQ-RZ-005：boards/useBoards/IndicatorManager/ChartWorkspace测试覆盖旧配置只改草稿、恢复仍修正、保存落库、无默认猜测关联、参数预算、切关联取消旧请求并重新查询、分页携带comparison和版本。

## 门禁

2026-09-21 完整执行 `bash scripts/verify.sh`，全部本地门禁通过：前端174测试、生产构建；文档契约、Go全量测试、race、vet、性能与容器配置安全检查。

- Go总覆盖率87.1%；market94.3%、indicator92.5%、strategy94.8%、backtest90.4%。
- 前端statements92.00%、branches85.28%、functions88.81%、lines94.15%。
- 全市场性能扫描171.6ms，snapshot p95 875ns。
- `--mysql`不适用：未修改SQL、库表/索引、事务、数据库驱动；JSON看板继续原读写路径。
- `--image`不适用：未改变依赖、打包、镜像或部署。

## 独立评审

`review_relative_zscore`只读review检查架构、遗留清理、错误隔离、因果与分页、公式及UI。发现P2：历史下界随before移动会改变EMA初始化。已固定为服务UTC当天向前20年的起点（不随before移动），新增跨20年与smooth500分页一致性测试及边界外空页测试，失败复现后修复。Reviewer独立重跑该回归通过，复查无阻塞问题，确认外部门禁不适用。

下界按服务UTC日期定义；跨UTC日继续查询会按新日期取20年范围，当前HTTP/应用契约已明确。本次未增加快照日期字段或数据库迁移。
