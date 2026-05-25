package data

import (
	"fmt"
	"time"

	"trading/config"
	"trading/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	defaultMaxOpenConns           = 25
	defaultMaxIdleConns           = 5
	defaultConnMaxLifetimeMinutes = 30
)

type Data struct {
	db *gorm.DB
}

// New 创建 Data 实例，内部根据配置初始化 gorm.DB 连接与连接池
func New(cfg config.DB) (*Data, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB failed: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = defaultMaxOpenConns
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = defaultMaxIdleConns
	}
	lifetimeMin := cfg.ConnMaxLifetimeMinutes
	if lifetimeMin <= 0 {
		lifetimeMin = defaultConnMaxLifetimeMinutes
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(lifetimeMin) * time.Minute)

	if err := db.AutoMigrate(&model.StockKlineDaily{}, &model.StockKlineWeekly{}, &model.FinancialReport{}, &model.StockInfo{}); err != nil {
		return nil, fmt.Errorf("auto migrate failed: %w", err)
	}

	return &Data{db: db}, nil
}

// DB 返回底层 gorm.DB 实例
func (d *Data) DB() *gorm.DB {
	return d.db
}

// StockKlineDaily 返回日线 Repository
func (d *Data) StockKlineDaily() StockKlineDailyRepo {
	return newStockKlineDailyRepo(d.db)
}

// StockKlineWeekly 返回周线 Repository
func (d *Data) StockKlineWeekly() StockKlineWeeklyRepo {
	return newStockKlineWeeklyRepo(d.db)
}

// FinancialReport 返回财报 Repository
func (d *Data) FinancialReport() FinancialReportRepo {
	return newFinancialReportRepo(d.db)
}

// StockInfo 返回股票信息 Repository
func (d *Data) StockInfo() StockInfoRepo {
	return newStockInfoRepo(d.db)
}
