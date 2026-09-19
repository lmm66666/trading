package data

import (
	"fmt"
	"time"

	"trading/config"
	mysqlinfra "trading/internal/infrastructure/mysql"
	"trading/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	defaultMaxOpenConns           = 60
	defaultMaxIdleConns           = 10
	defaultConnMaxLifetimeMinutes = 30
)

type Data struct {
	db *gorm.DB
}

// New 创建 Data 实例，内部根据配置初始化 gorm.DB 连接与连接池
func New(cfg config.DB) (*Data, error) {
	db, err := gorm.Open(mysql.Open(mysqlDSN(cfg)), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql failed: %w", err)
	}
	return initializeData(cfg, db, migrateSchema)
}

// initializeData 在初始化成功前拥有连接；任何迁移错误都统一清理，
// 成功返回后才把连接所有权交给 Data。
func initializeData(cfg config.DB, db *gorm.DB, migrate func(*gorm.DB) error) (*Data, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB failed: %w", err)
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = sqlDB.Close()
		}
	}()

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
	if err := migrate(db); err != nil {
		return nil, err
	}
	transferred = true
	return &Data{db: db}, nil
}

func migrateSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(runtimeModels()...); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}
	if err := mysqlinfra.Migrate(db); err != nil {
		return fmt.Errorf("migrate strategy kernel schema: %w", err)
	}
	return nil
}

// runtimeModels 仅保留旧库迁移链路依赖的 t_stock_info（迁移器 stage "info"
// 读取证券名称）；旧 K 线表由迁移工具自行访问，正常启动不创建、不变更其结构。
func runtimeModels() []any {
	return []any{&model.StockInfo{}}
}

// mysqlDSN 统一无时区 DATETIME 的编码与解析口径；旧表交易日期是字符串字段，
// 其内容不会被驱动按时区转换。
func mysqlDSN(cfg config.DB) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=UTC",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
}

// DB 返回底层 gorm.DB 实例
func (d *Data) DB() *gorm.DB {
	return d.db
}
