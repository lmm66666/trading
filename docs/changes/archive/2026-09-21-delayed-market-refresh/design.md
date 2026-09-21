---
id: CHG-2026-09-21-DELAYED-MARKET-REFRESH-DESIGN
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T12:28:17+08:00"
approved_revision: "sha256:ee246a2e0d53b18c894c0145aec3177f1f8204d81ec97830636a9ff2b2fc6cfb"
approved_scope: ["REQ-DELAY-001", "REQ-DELAY-002", "REQ-DELAY-003"]
---

# 首轮等待设计

## 当前与目标

现有 MarketScheduler.Start 与 FuturesScheduler.Start 在每轮先调用 RunOnce、再等待 ticker，因此启动立即更新。目标仅调换这两步：创建原 ticker 后先 select 等待 ctx.Done 或 ticker.C，收到 tick 后运行原 RunOnce 和错误分类，再进入下一轮。

保持 time.NewTicker 固定节拍、同进程无重叠、逐证券失败继续、全局错误退出等语义。股票默认首次等待 24 小时，期货等待已有配置间隔；重启重新计时。手动股票 TriggerNow 与 HTTP 路径保持原样，可在首轮等待中立即执行，且不改变 ticker。

## 数据与边界

不新增表、索引、SQL、接口、配置或外部依赖；库中已有 COMPLETE 行情仍可查看，首次空库不会因为服务启动自动获取行情。没有新增期货手动刷新能力。等待与取消不会访问行情源或写数据库。

## 验证与文档

用 Go testing/synctest 虚拟时间验证两类调度首轮/后续周期、首轮前取消、手动股票触发不改变定时节拍。调整旧测试中依赖“启动立即运行”的假设，保留原 RunOnce/并发测试。运行 Go 测试、覆盖率、Race、编译与文档检查；按本仓库要求独立评审。没有改变数据库和镜像语义，两项外部门禁不适用。

更新应用层当前设计、系统部署说明、运行手册及 AGENTS 部署背景；历史 NAS 拆分记录保留当时行为，不回写历史。用户直接指令是该设计唯一语义变化的批准依据。
