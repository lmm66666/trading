package mysql

import "time"

type BacktestTradeModel struct {
	BaseModel
	RunID         string    `gorm:"type:varbinary(64);not null;uniqueIndex:uq_trade_sequence,priority:1"`
	Sequence      uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_trade_sequence,priority:2"`
	OrderSequence uint64    `gorm:"type:bigint unsigned;not null"`
	FillID        string    `gorm:"type:longblob;not null"`
	OrderID       string    `gorm:"type:longblob;not null"`
	Time          time.Time `gorm:"type:datetime(6);not null"`
	Side          string    `gorm:"size:8;not null"`
	Quantity      int64     `gorm:"type:bigint;not null"`
	Price         int64     `gorm:"type:bigint;not null"`
	Gross         int64     `gorm:"type:bigint;not null"`
	Commission    int64     `gorm:"type:bigint;not null"`
	StampDuty     int64     `gorm:"type:bigint;not null"`
	TransferFee   int64     `gorm:"type:bigint;not null"`
}

func (BacktestTradeModel) TableName() string { return "t_backtest_trades" }
