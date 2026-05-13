package model

// ExchangeRate 汇率实时数据
type ExchangeRate struct {
	Code          string  `json:"code"`           // 代码，如 USDCNY
	Name          string  `json:"name"`           // 显示名称，如 美元/人民币
	Open          float64 `json:"open"`           // 开盘价
	Now           float64 `json:"now"`            // 最新价
	ChangePercent float64 `json:"change_percent"` // 涨跌幅（百分比）
}
