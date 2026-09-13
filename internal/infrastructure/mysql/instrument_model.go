package mysql

type InstrumentModel struct {
	BaseModel
	Exchange string `gorm:"size:8;not null;uniqueIndex:uq_instrument,priority:1"`
	Code     string `gorm:"size:6;not null;uniqueIndex:uq_instrument,priority:2"`
	Name     string `gorm:"size:128;not null"`
	Board    string `gorm:"size:32;not null"`
	Active   bool   `gorm:"not null;index:idx_instrument_active"`
	LotSize  int64  `gorm:"type:bigint;not null"`
	Source   string `gorm:"type:varbinary(128);not null"`
}

func (InstrumentModel) TableName() string { return "t_instruments" }
