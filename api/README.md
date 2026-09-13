# api 规范

## 通用规定
1. 一个 api 接口一个文件，文件命名为接口名称，并用下划线连接。比如 GetStockAnalysisData --> get_stock_analysis_data.go。
2. V1 策略接口通过 `KernelServices` 注入应用服务及只读 port，禁止导入 GORM 或调用旧技术策略业务。创建请求使用显式 DTO、Validate、1MiB 请求体限制，拒绝未知字段和多余 JSON；所有调用传递请求 context。
3. Run 输出使用安全 DTO，不返回 RequestJSON、幂等键、租约 owner/token 或底层错误。状态、错误码和 curl 示例以 `api.md` 为准。
4. 明细分页 limit 为1–1000（默认100），使用排他 `after_sequence`。快照首响应必须返回 SnapshotID 与可继续游标；后续绑定该身份，不能重新查询最新版本。
5. 旧 signal 只读完整的单个快照；旧 backtest 精确解析活跃证券、创建持久化 Run，并在有界同步等待后返回结果或202。旧路径不存在技术引擎回退。
