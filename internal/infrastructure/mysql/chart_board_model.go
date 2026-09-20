package mysql

type ChartBoardModel struct {
	BaseModel
	Name     string `gorm:"size:40;not null"`
	Config   string `gorm:"type:json;not null"`
	IsActive bool   `gorm:"column:is_active;not null;default:0"`
}

func (ChartBoardModel) TableName() string { return "t_chart_boards" }
