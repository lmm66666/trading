package mysql

type WatchlistModel struct {
	BaseModel
	Exchange string `gorm:"size:8;not null;uniqueIndex:uq_watchlist,priority:1"`
	Code     string `gorm:"size:16;not null;uniqueIndex:uq_watchlist,priority:2"`
}

func (WatchlistModel) TableName() string { return "t_watchlist" }
