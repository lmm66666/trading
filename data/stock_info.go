package data

import (
	"context"

	"trading/model"

	"gorm.io/gorm"
)

// StockInfoRepo 股票信息仓库接口
type StockInfoRepo interface {
	// FindByCode 按代码查询股票名称
	FindByCode(ctx context.Context, code string) (*model.StockInfo, error)
	// FindAll 查询所有股票信息
	FindAll(ctx context.Context) ([]*model.StockInfo, error)
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, infos []*model.StockInfo) error
}

type stockInfoRepo struct {
	db *gorm.DB
}

func newStockInfoRepo(db *gorm.DB) StockInfoRepo {
	return &stockInfoRepo{db: db}
}

func (r *stockInfoRepo) FindByCode(ctx context.Context, code string) (*model.StockInfo, error) {
	var info model.StockInfo
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&info).Error; err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *stockInfoRepo) FindAll(ctx context.Context) ([]*model.StockInfo, error) {
	var infos []*model.StockInfo
	if err := r.db.WithContext(ctx).Find(&infos).Error; err != nil {
		return nil, err
	}
	return infos, nil
}

func (r *stockInfoRepo) SaveBatch(ctx context.Context, infos []*model.StockInfo) error {
	return r.db.WithContext(ctx).Save(infos).Error
}
