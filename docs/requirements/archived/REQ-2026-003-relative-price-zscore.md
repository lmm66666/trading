# REQ-2026-003：相对价格滚动 Z-Score

| 属性 | 内容 |
|---|---|
| 状态 | 已完成 |
| 创建日期 | 2026-09-14 |
| 适用范围 | `internal/indicator` |
| 目标版本 | 相对价格指标第一版 |

## 1. 背景与问题

`investment-research` 中已有 TradingView Pine 与 Python 参考实现，用于观察两条相关价格的对数价格比偏离其滚动常态的程度。`trading` 的 Go 指标模块尚无等价的纯计算函数，调用方无法在不引入 Pine 或 Python 的情况下复用这一计算口径。

本需求只把已确认的数学口径用 Go 独立实现。原 Pine 与 Python 文件保持不变，不迁移源码，也不把跨证券读取引入现有单证券指标图。

## 2. 目标

1. 在 `internal/indicator` 提供 `RelativePriceZScore(primaryPrices, comparisonPrices []float64, period int) (Series, error)`。
2. 使用两条已按日线时点对齐的正价格序列，计算对数价格比的滚动总体标准差和 Z-Score。
3. 通过现有 `Series` 表达预热不足与零方差窗口的无效值。
4. 保持指标模块覆盖率不低于 90%，全仓覆盖率不低于 80%。

## 3. 非目标

1. 不修改 `/Users/lmm/Desktop/investment-research` 中的 Pine、Python、测试或研究文档。
2. 不新增 `indicator.Ref`，不接入现有 `Build` 计算图。
3. 不修改图表 API、Web、策略、扫描、回测、行情仓储或数据库。
4. 不负责两条序列的行情读取、复权、汇率换算或交易日对齐。
5. 不检验相关性或协整，不把偏离解释为均值回归或交易信号。

## 4. 当前行为

`internal/indicator` 当前只在一个固定行情 Dataset 上构建 OHLC、SMA、EMA、成交量均线、MACD 和 KDJ。`Build` 和 `Ref` 都不表达第二个证券，策略时间线也只允许同一证券的不同周期。

## 5. 目标行为

调用方把索引相同的两个元素视为同一日线观察值。函数按以下公式计算：

```text
spread[i] = ln(primaryPrices[i]) - ln(comparisonPrices[i])
z[i] = (spread[i] - populationMean(window)) / populationStdDev(window)
```

- 标准差使用总体口径，分母为窗口长度 `N`，与 Pine `ta.stdev(source, length, true)` 一致。
- 输出长度与输入长度一致；前 `period-1` 个点无效。
- `period` 大于输入长度时返回同长度的全无效 Series，不返回错误。
- 窗口总体标准差不大于 `1e-12` 时该点无效，不用零或非有限值伪装结果。
- 输入长度不同、`period < 2`、任一价格非正或非有限时返回可由 `errors.Is` 识别的输入错误。
- 函数只读输入切片，追加未来价格不能改变既有前缀结果。

## 6. 影响矩阵

| 模块或文档 | 影响 | 必需变更 |
|---|---|---|
| `internal/indicator/DESIGN.md` | 新增独立的双序列纯计算能力 | 记录接口、计算口径、无效值和失败语义 |
| `internal/indicator` | 新增导出函数和输入错误 | 增加实现与单元测试 |
| 其他模块 | 无运行时影响 | 不修改 |

## 7. 方案比较与选择

选择在 `internal/indicator` 增加独立纯函数。它复用现有 `Series` 的有效位语义，同时不扩大单 Dataset 的 `Ref` 和 `Build` 契约。

放弃直接复制 Python 实现，因为这会只为一个小型公式引入第三套运行时和测试入口。放弃立即接入指标图、API 或策略，因为跨证券输入需要额外定义版本绑定、交易日对齐和加载上限，超出当前调用需求。

## 8. 接口、数据与兼容性

本需求只增加 Go 包级接口，不改变 HTTP API、持久化模型、数据版本、策略版本或引擎版本。调用方继续拥有输入切片；函数不会修改或保存数据。现有调用者不受影响，无需迁移和兼容层。

## 9. 风险与失败处理

- 误用样本标准差会偏离 Pine 结果，由手工推导的三点窗口测试防止。
- 未对齐的日期无法由裸切片识别，因此接口文档明确把日线时点对齐设为调用方前置条件。
- 固定比价会产生零方差，结果保持无效，避免形成虚假零偏离。
- 实现必须保持有界计算，不增加外部依赖、并发、日志或持久化。

## 10. 测试与验收标准

1. 手工推导案例证明使用总体标准差。
2. 测试覆盖比例缩放不变性、预热、固定比价、非法输入、输入不变和前缀不变性。
3. `go test ./internal/indicator -cover -count=1` 通过且覆盖率不低于 90%。
4. `npm --prefix web run check && go test ./... && go vet ./...` 通过。
5. `bash scripts/verify.sh` 通过；环境不可用的门禁必须准确记录。

## 11. 开发中设计修订

无。

## 12. 最终验收结果

实现、测试和独立代码复核均已完成：

- `go test ./internal/indicator -cover -count=1` 通过，指标模块覆盖率为 91.3%。
- `go test ./internal/indicator -run '^TestRelativePriceZScore' -count=100` 通过。
- `go test -race ./internal/indicator -count=1` 通过。
- `npm --prefix web run check`、`go test ./...` 和 `go vet ./...` 均通过；全仓 Go 覆盖率为 85.6%。
- `bash scripts/verify.sh` 的前端、Go 测试、覆盖率、竞态检查、静态检查和行情性能门禁通过；MySQL 8.0 容器创建在 180 秒后超时，用户明确决定跳过该集成测试，因此 MySQL 5.7/8.0 完整集成门禁未验证。
- 单独执行 Docker 镜像构建时，Docker Hub 匿名认证请求网络超时，镜像门禁未验证。
- 独立代码复核确认非法输入双腿对称覆盖和标准差阈值边界测试完整，未发现代码阻塞问题。

未验证项均属于交付环境门禁，不改变本需求的纯函数实现范围；不得将其表述为已通过。

## 13. 代码—设计冲突与用户裁决

无。
