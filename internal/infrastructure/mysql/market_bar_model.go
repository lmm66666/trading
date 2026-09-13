package mysql

import "time"

type MarketBarModel struct {
	BaseModel
	InstrumentID     uint64    `gorm:"type:bigint unsigned;not null;index:idx_bar_current,priority:3;index:idx_bar_instrument,priority:1;uniqueIndex:uq_bar_revision,priority:1;index:idx_bar_dirty,priority:2"`
	Timeframe        string    `gorm:"size:16;not null;index:idx_bar_current,priority:1;index:idx_bar_instrument,priority:2;uniqueIndex:uq_bar_revision,priority:2"`
	OpenTime         time.Time `gorm:"type:datetime(6);not null"`
	CloseTime        time.Time `gorm:"type:datetime(6);not null;index:idx_bar_current,priority:2;index:idx_bar_instrument,priority:3;uniqueIndex:uq_bar_revision,priority:3"`
	Revision         uint32    `gorm:"type:int unsigned;not null;uniqueIndex:uq_bar_revision,priority:4"`
	ValidFromVersion uint64    `gorm:"type:bigint unsigned;not null;index:idx_bar_dirty,priority:1"`
	ValidToVersion   *uint64   `gorm:"type:bigint unsigned;index:idx_bar_valid_to"`
	Open             int64     `gorm:"type:bigint;not null"`
	High             int64     `gorm:"type:bigint;not null"`
	Low              int64     `gorm:"type:bigint;not null"`
	Close            int64     `gorm:"type:bigint;not null"`
	Volume           int64     `gorm:"type:bigint;not null"`
	Amount           int64     `gorm:"type:bigint;not null"`
	TradingStatus    string    `gorm:"size:16;not null"`
	LimitUp          *int64    `gorm:"type:bigint"`
	LimitDown        *int64    `gorm:"type:bigint"`
}

func (MarketBarModel) TableName() string { return "t_market_bars" }
