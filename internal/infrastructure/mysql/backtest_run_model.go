package mysql

type BacktestRunModel struct {
	BaseModel
	RunID              string `gorm:"size:64;not null;uniqueIndex:uq_backtest_run"`
	InstrumentID       uint64 `gorm:"type:bigint unsigned;not null;index:idx_backtest_instrument"`
	StrategyID         string `gorm:"size:64;not null"`
	StrategyVersion    string `gorm:"size:32;not null"`
	EngineVersion      string `gorm:"size:32;not null"`
	DataVersion        uint64 `gorm:"type:bigint unsigned;not null"`
	ParametersJSON     []byte `gorm:"type:json;not null"`
	ConfigJSON         []byte `gorm:"type:json;not null"`
	TotalReturn        *float64
	AnnualizedReturn   *float64
	MaximumDrawdown    float64 `gorm:"not null"`
	ClosedTrades       uint64  `gorm:"type:bigint unsigned;not null"`
	WinRate            *float64
	ProfitFactor       *float64
	AverageHoldingBars *float64
	HasOpenPosition    bool `gorm:"not null"`
}

func (BacktestRunModel) TableName() string { return "t_backtest_runs" }
