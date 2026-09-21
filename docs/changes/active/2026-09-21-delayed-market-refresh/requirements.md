---
id: CHG-2026-09-21-DELAYED-MARKET-REFRESH
status: implementing
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T12:28:17+08:00"
approved_revision: "sha256:aa8c856235d8a71416142bdafdbf57623c8ab3b7fd2ca8dbaae86a16eb5234a6"
approved_scope: ["REQ-DELAY-001", "REQ-DELAY-002", "REQ-DELAY-003"]
---

# 更新服务启动不立即采集

## 用户决定与范围

用户明确要求“更新服务启动就不立即跑了吧。如果需要立即跑也可以通过后端调用”。本变更仅落实这一确定行为：股票、期货 Start 启动后先等待原 interval；手动股票刷新仍立即执行，不重置定时周期。更新周期、范围、限频、存储和接口均不改变，不增加立即启动兼容开关。

## 需求与验收

- REQ-DELAY-001：启动后首个 interval 到达前，股票、期货均不自动采集；首个周期及后续周期到达后正常执行。
- REQ-DELAY-002：等待首轮期间可通过现有股票手动刷新立即更新，定时触发时间不重置。期货不新增手动接口。
- REQ-DELAY-003：等待期间取消应及时退出、不触发采集；保留拒绝重复 Start、禁止重叠和错误传播行为。

## 批准依据

本次用户指令直接批准上述首次触发语义调整，未批准其他行为变化。当前设计的“启动立即执行”由此范围覆盖；[目标设计](design.md) 与指令同范围。生产实现前记录该内容摘要，验收见 [验证](verification.md)。
