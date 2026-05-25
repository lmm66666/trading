package model

import "gorm.io/gorm"

// StockKline 股票K线数据
type StockKline struct {
	gorm.Model
	Code    string  `gorm:"size:16;index;uniqueIndex:idx_code_date" json:"code"` // 股票代码，格式统一为 600312，不是 sh600312
	Date    string  `gorm:"size:20;uniqueIndex:idx_code_date" json:"date"`       // 日期，格式统一为 2022-01-22
	Open    float64 `gorm:"type:decimal(16,4)" json:"open"`
	High    float64 `gorm:"type:decimal(16,4)" json:"high"`
	Low     float64 `gorm:"type:decimal(16,4)" json:"low"`
	Close   float64 `gorm:"type:decimal(16,4)" json:"close"`
	Volume  int64   `gorm:"type:bigint" json:"volume"`
	AuxMA20 float64 `gorm:"-" json:"-"`                                          // 跨周期日 20 日均线，仅内存使用
}
