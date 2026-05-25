package data

import (
	"context"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"trading/model"
)

// StockKlineWeeklyRepo 定义 StockKlineWeekly 数据访问接口
type StockKlineWeeklyRepo interface {
	Create(ctx context.Context, kline *model.StockKlineWeekly) error
	CreateBatch(ctx context.Context, klines []*model.StockKlineWeekly) error
	Upsert(ctx context.Context, klines []*model.StockKlineWeekly) error
	FindByID(ctx context.Context, id uint) (*model.StockKlineWeekly, error)
	FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error)
	FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineWeekly, error)
	FindLatestByCode(ctx context.Context, code string) (*model.StockKlineWeekly, error)
	FindAllCodes(ctx context.Context) ([]string, error)
	FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineWeekly, error)
	Update(ctx context.Context, kline *model.StockKlineWeekly) error
	Delete(ctx context.Context, id uint) error
	List(ctx context.Context, limit, offset int) ([]*model.StockKlineWeekly, error)
}

type stockKlineWeeklyRepo struct {
	db *gorm.DB
}

func newStockKlineWeeklyRepo(db *gorm.DB) StockKlineWeeklyRepo {
	return &stockKlineWeeklyRepo{db: db}
}

// Create 插入单条周线数据
func (r *stockKlineWeeklyRepo) Create(ctx context.Context, kline *model.StockKlineWeekly) error {
	return r.db.WithContext(ctx).Create(kline).Error
}

// CreateBatch 批量插入周线数据
func (r *stockKlineWeeklyRepo) CreateBatch(ctx context.Context, klines []*model.StockKlineWeekly) error {
	return r.db.WithContext(ctx).CreateInBatches(klines, 100).Error
}

// Upsert 批量插入或更新周线数据（code+date 联合唯一）
func (r *stockKlineWeeklyRepo) Upsert(ctx context.Context, klines []*model.StockKlineWeekly) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code"}, {Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume", "updated_at"}),
	}).CreateInBatches(klines, 100).Error
}

// FindByID 根据主键查询周线
func (r *stockKlineWeeklyRepo) FindByID(ctx context.Context, id uint) (*model.StockKlineWeekly, error) {
	var kline model.StockKlineWeekly
	if err := r.db.WithContext(ctx).First(&kline, id).Error; err != nil {
		return nil, err
	}
	return &kline, nil
}

// FindByCodeWithPagination 根据股票代码分页查询周线，按日期降序返回
func (r *stockKlineWeeklyRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineWeekly, error) {
	var klines []*model.StockKlineWeekly
	if err := r.db.WithContext(ctx).Where("code = ?", code).Order("date DESC").Limit(limit).Offset(offset).Find(&klines).Error; err != nil {
		return nil, err
	}
	return klines, nil
}

// FindByCode 根据股票代码查询最近 limit 条周线，按日期升序返回
func (r *stockKlineWeeklyRepo) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error) {
	var klines []*model.StockKlineWeekly
	query := r.db.WithContext(ctx).Where("code = ?", code).Order("date DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&klines).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}
	return klines, nil
}

// Update 更新周线数据
func (r *stockKlineWeeklyRepo) Update(ctx context.Context, kline *model.StockKlineWeekly) error {
	return r.db.WithContext(ctx).Save(kline).Error
}

// Delete 软删除指定周线记录
func (r *stockKlineWeeklyRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.StockKlineWeekly{}, id).Error
}

// FindLatestByCode 查询指定代码最新一条周线（按日期降序）
func (r *stockKlineWeeklyRepo) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineWeekly, error) {
	var kline model.StockKlineWeekly
	if err := r.db.WithContext(ctx).Where("code = ?", code).Order("date DESC").First(&kline).Error; err != nil {
		return nil, err
	}
	return &kline, nil
}

// FindAllCodes 查询所有 distinct 的股票代码
func (r *stockKlineWeeklyRepo) FindAllCodes(ctx context.Context) ([]string, error) {
	var codes []string
	if err := r.db.WithContext(ctx).Model(&model.StockKlineWeekly{}).Distinct("code").Pluck("code", &codes).Error; err != nil {
		return nil, err
	}
	return codes, nil
}

// FindRecentByCodes 批量查询多只股票的最近 limit 条周线，按 code 分组返回（日期升序）
// 每个 code 独立并发查询，SQL 使用 WHERE code = ? + LIMIT，直接走 (code, date) 索引前缀
func (r *stockKlineWeeklyRepo) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineWeekly, error) {
	if len(codes) == 0 || limit <= 0 {
		return make(map[string][]*model.StockKlineWeekly), nil
	}

	const queryConcurrency = 40

	result := make(map[string][]*model.StockKlineWeekly)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, queryConcurrency)

	for _, code := range codes {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()

			if err := ctx.Err(); err != nil {
				return
			}

			sem <- struct{}{}
			defer func() { <-sem }()

			var klines []*model.StockKlineWeekly
			if err := r.db.WithContext(ctx).
				Where("code = ?", c).
				Order("date DESC").
				Limit(limit).
				Find(&klines).Error; err != nil {
				return
			}

			// 反转为日期升序
			for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
				klines[i], klines[j] = klines[j], klines[i]
			}

			mu.Lock()
			result[c] = klines
			mu.Unlock()
		}(code)
	}
	wg.Wait()

	return result, nil
}

// List 分页查询所有周线数据
func (r *stockKlineWeeklyRepo) List(ctx context.Context, limit, offset int) ([]*model.StockKlineWeekly, error) {
	var klines []*model.StockKlineWeekly
	if err := r.db.WithContext(ctx).Limit(limit).Offset(offset).Find(&klines).Error; err != nil {
		return nil, err
	}
	return klines, nil
}
