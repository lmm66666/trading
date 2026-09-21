---
id: CHG-UPDATER-EMBEDDED-CONFIG-VERIFICATION
result: pending
authority: evidence
---

# 验证记录

[需求](requirements.md)与[设计](design.md)。MySQL 门禁不适用：应用代码、数据库语义与迁移均未改变，没有连接业务库。

- 本地门禁通过：前端 235 个测试，覆盖率语句 93.59%、分支 87.73%、函数 90.54%、行 96.03%；Go 总覆盖率 85.8%，market 94.3%、indicator 92.5%、strategy 94.8%、backtest 90.4%；全量测试、Race、vet、性能及文档检查通过。
- `scripts/verify.sh --image`：镜像构建进行中，日志 `/tmp/updater-embedded-verify.log`。
- REQ-EC-001/003：真实配置镜像 `trading-updater:20260921-configured` 已构建并离线检查 linux/amd64、app 用户、0400 权限、默认 ENTRYPOINT/CMD 和内置配置与用户源文件逐字节一致，通过。检查使用 `--network none`，未启动应用。
- REQ-EC-002：普通镜像隔离、连续更换 secret 配置及缺失输入负向测试进行中。
- 独立子 Agent 评审没有生产实现阻塞问题；建议补默认命令断言及 configured 构建代理参数，均已修复并复审通过。最终 `TestVerification` 与 shell 语法检查通过。

真实配置和 tar 未提交。导出 `/Users/lmm/Desktop/trading-updater-20260921-configured/trading-updater-configured-linux-amd64.tar`，34,202,624 字节；SHA-256 `4a1a2f2e28101bac94078205ba2fa23dae31b1dedd04d37fe5b0af8acf8b9d19`。最终 tar 有意包含凭据，仅用于用户私有 NAS。未执行 NAS 部署或采集验收。
