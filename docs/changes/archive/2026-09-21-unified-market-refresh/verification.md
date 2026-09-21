---
id: CHG-2026-09-21-UNIFIED-MARKET-REFRESH-VERIFICATION
result: passed
authority: evidence
---

# 验证

生命周期见 [需求](requirements.md)，目标见 [设计](design.md)。

| 需求 | 设计 | 实现 | 测试 | 结果 |
|---|---|---|---|---|
| REQ-U01/U02 | 统一触发与生命周期 | application/refresh_all.go、futures_scheduler.go、main.go、updater.go、api/updater_client.go | TestUnifiedRefresh*、TestFuturesManualRefresh*、TestRemoteRefresh* | passed |
| REQ-U03 | 前端 | RefreshMonitor、API client、updates.css | 25项面板回归，含单按钮/独立回执/旧能力恢复/定位 | passed |
| REQ-U04 | 空记录 | RefreshProgressStore | TestRefreshMissingRecordsDoNotLogErrors、原排序和持久化回归 | passed |

## 检查与覆盖率

- 基线：main 4c27777；origin fetch 成功，main 包含 origin/main，未推送。
- 红绿证据：原面板13项基线通过；新定位/错误分类测试失败后通过；统一回执 UI 5项失败后通过；新增调度测试先报告缺失方法，再实现通过；代理旧解析拒绝联合回执，修复后通过；真实 MySQL 空记录测试先抓到 record not found 日志，修复后通过。旧 DISABLED 回执边界测试也先失败再通过。
- `bash scripts/verify.sh` passed：前端全量、构建、Go 全量测试、vet；最终前端小修后 `npm --prefix web run check` 再验通过（27文件247测试）。
- `npm --prefix web run check` passed：语句93.73%、分支87.98%、函数90.66%、行96.13%；更新面板约96%语句覆盖率，生产构建成功（约503kB主包已有规模警告，非阻断）。
- `go test ./api ./internal/application ./internal/port -coverprofile=...` passed：API85.9%、application90.4%、port94.4%，合计89.5%；TriggerMarketRefresh、refreshOutcome、新期货TriggerTracked/Wait及回执校验均100%。核心行情/指标/策略/回测未改动，按工程标准不重跑无关全量覆盖率。
- `go test -race ./internal/application ./api .` passed。
- `go test -tags=integration ./internal/infrastructure/mysql -run '^TestRefresh(Missing|Latest|Progress)' -coverprofile=... -count=1` passed，真实 MySQL 随机隔离库；本次修改的 GetRefreshRun/LatestRefreshRun 均85.7%。仅选择更新存储测试的整个 MySQL 包覆盖率不作为全包覆盖率声明。
- `bash scripts/verify.sh --mysql` passed：MySQL8.4/x86_64兼容检查、完整MySQL隔离集成（237.837秒）、dbtest、data全部通过。
- 独立子 Agent review 完成：曾发现旧 DISABLED 回执遮蔽最新 true，已修复并复核，当前无待修复项。架构、防重、根context、退出Wait、回执和空日志符合批准设计。
- 浏览器视觉检查 not-required：按用户规则未调用浏览器；组件测试覆盖定位、窄屏避让、外部关闭、Escape焦点归还，未宣称完成实机视觉验收。
- 镜像检查 not-required：未改镜像/配置打包；并非免除部署升级。

## 环境边界

当前实际 NAS 更新服务不可达；本次验证不触发业务库行情采集。交付代码须同步升级 NAS updater 与本地工作台后使用，不能将空历史200或模拟受理解释为 NAS 已恢复。无数据迁移，不改业务库或本地配置。

实现内容摘要（上述受影响生产源码按矩阵顺序聚合）：sha256:555d7ed8d63bc9dbc017c8640d2ec20dd8460b61451a88104adaebc69349dd1a。
