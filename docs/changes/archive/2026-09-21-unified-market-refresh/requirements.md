---
id: CHG-2026-09-21-UNIFIED-MARKET-REFRESH
status: implemented
approval_status: approved
authority: normative
approved_by: user
approved_at: "2026-09-21T23:58:06+08:00"
approved_revision: "sha256:7f1ce1ba342f6c3d264ff0af7510ca4d4217a059a5fb89cd3ceedfaf01c2c1a7"
approved_scope: [REQ-U01, REQ-U02, REQ-U03, REQ-U04]
---

# 统一行情更新需求

## 场景与容量
个人 NAS 单 updater、本机单 workbench；手动按需，沿用股票 24 小时与期货配置周期，启动不立即采集。范围为现有全市场活跃股票（端口上限）及启用的八个期货连续品种；共享不低于五秒一次的来源限频。股票沿用最近 20 根重叠窗口、期货全历史，不新增“当天更新过即跳过”。

## 需求与验收
- REQ-U01：面板一个“立即更新”按钮触发股票与已启用期货；已有任务的类别不重复启动，另一类正常启动，禁用期货明确跳过。
- REQ-U02：分别返回、展示各类别受理/运行中/未启用/未受理状态，已受理任务独立于页面和请求生命周期，退出 updater 等待它们停止。
- REQ-U03：面板锚定入口、视口避让，股票/期货为进度展示，错误区分不可达/超时/响应异常，未知结果不伪造受理、不自动重发。
- REQ-U04：空历史与未知任务保留现有空值/404 契约，不误报 record not found；真实数据库错误仍传播。

## 范围及兼容
直接更改原无证券身份批量 POST 的含义及回执；前端、workbench 与 NAS updater 同步升级，不保留旧批量接口兼容。单股票同步刷新、调度周期、采集范围、数据版本与数据库模型保持原规则。NAS 不可达需明确反馈，不以 UI 修复冒充实际部署恢复。

## 批准证据
用户本任务回复“按这个范围实施”，明确批准统一股票/已启用期货、运行中防重、沿用采集规则、同步升级且不兼容旧批量接口；前一回复“你都修一下吧”批准面板与日志修复。内容版本 unified-refresh-v1，文档摘要见 approved_revision。验证见 [验证](verification.md)。
