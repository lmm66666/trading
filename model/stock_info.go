package model

import "gorm.io/gorm"

// StockInfo 股票代码到名称的映射
type StockInfo struct {
	gorm.Model
	Code string `gorm:"size:16;index;uniqueIndex:idx_code" json:"code"` // 股票代码
	Name string `gorm:"size:32" json:"name"`                             // 股票名称
}

func (StockInfo) TableName() string {
	return "t_stock_info"
}
