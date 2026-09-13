package mysql

import "time"

type BacktestEquityModel struct {
	BaseModel
	RunID         string    `gorm:"type:varbinary(64);not null;uniqueIndex:uq_equity_sequence,priority:1"`
	Sequence      uint64    `gorm:"type:bigint unsigned;not null;uniqueIndex:uq_equity_sequence,priority:2"`
	Time          time.Time `gorm:"type:datetime(6);not null"`
	Cash          int64     `gorm:"type:bigint;not null"`
	PositionValue int64     `gorm:"type:bigint;not null"`
	Equity        int64     `gorm:"type:bigint;not null"`
	Drawdown      float64   `gorm:"not null"`
}

func (BacktestEquityModel) TableName() string { return "t_backtest_equity_points" }
