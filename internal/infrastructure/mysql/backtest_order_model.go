package mysql

import "time"

type BacktestOrderModel struct {
	BaseModel
	RunID        string     `gorm:"type:varbinary(64);not null;uniqueIndex:uq_order_sequence,priority:1"`
	Sequence     uint64     `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_order_sequence,priority:2"`
	OrderID      string     `gorm:"type:longblob;not null"`
	CreatedTime  time.Time  `gorm:"type:datetime(6);not null"`
	AttemptedAt  *time.Time `gorm:"type:datetime(6)"`
	Reason       string     `gorm:"type:text;not null"`
	Side         string     `gorm:"size:8;not null"`
	CreatedAtBar int64      `gorm:"type:bigint;not null"`
	ActiveAtBar  int64      `gorm:"type:bigint;not null"`
	Quantity     int64      `gorm:"type:bigint;not null"`
	Status       string     `gorm:"size:32;not null"`
	FinalReason  string     `gorm:"type:text;not null"`
}

func (BacktestOrderModel) TableName() string { return "t_backtest_orders" }
