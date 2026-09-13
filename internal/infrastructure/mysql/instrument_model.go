package mysql

import "time"

type InstrumentModel struct {
	BaseModel
	Exchange           string     `gorm:"size:16;not null;uniqueIndex:uq_instrument,priority:1"`
	Code               string     `gorm:"size:32;not null;uniqueIndex:uq_instrument,priority:2"`
	AssetClass         string     `gorm:"size:16;not null;index:idx_instrument_class_kind,priority:1"`
	InstrumentKind     string     `gorm:"size:32;not null;index:idx_instrument_class_kind,priority:2"`
	ProductCode        string     `gorm:"size:8;not null;index:idx_instrument_product_delivery,priority:1"`
	DeliveryMonth      *time.Time `gorm:"type:date;index:idx_instrument_product_delivery,priority:2"`
	LastTradeDate      *time.Time `gorm:"type:date"`
	ContractMultiplier int64      `gorm:"type:bigint;not null"`
	TickSize           int64      `gorm:"type:bigint;not null"`
	Name               string     `gorm:"size:128;not null"`
	Board              string     `gorm:"size:32;not null"`
	Active             bool       `gorm:"not null;index:idx_instrument_active"`
	LotSize            int64      `gorm:"type:bigint;not null"`
	Source             string     `gorm:"type:varbinary(128);not null"`
}

func (InstrumentModel) TableName() string { return "t_instruments" }
