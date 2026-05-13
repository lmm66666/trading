package model

// ShiborData Shibor 利率数据
type ShiborData struct {
	ReportDate   string  `json:"report_date"`   // 报告日期
	ReportPeriod string  `json:"report_period"` // 期限，如"隔夜(O/N)"
	IRRate       float64 `json:"ir_rate"`       // 利率值
	ChangeRate   float64 `json:"change_rate"`   // 变化点数（基点）
	IndicatorID  string  `json:"indicator_id"`  // 指标ID
}
