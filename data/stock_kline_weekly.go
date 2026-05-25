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
// 内部按 batchSize 拆分并发查询，通过 queryConcurrency 控制最大并发 SQL 数
func (r *stockKlineWeeklyRepo) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineWeekly, error) {
	if len(codes) == 0 || limit <= 0 {
		return make(map[string][]*model.StockKlineWeekly), nil
	}

	const (
		batchSize        = 20
		queryConcurrency = 30
	)

	// 拆分为 batches
	var batches [][]string
	for i := 0; i < len(codes); i += batchSize {
		end := i + batchSize
		if end > len(codes) {
			end = len(codes)
		}
		batches = append(batches, codes[i:end])
	}

	type batchResult struct {
		klist []*model.StockKlineWeekly
		err   error
	}

	results := make([]batchResult, len(batches))
	var wg sync.WaitGroup
	sem := make(chan struct{}, queryConcurrency)

	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, b []string) {
			defer wg.Done()

			if err := ctx.Err(); err != nil {
				results[idx] = batchResult{err: err}
				return
			}

			sem <- struct{}{}
			defer func() { <-sem }()

			var klines []*model.StockKlineWeekly
			err := r.db.WithContext(ctx).Where("code IN ?", b).Order("code, date DESC").Find(&klines).Error
			results[idx] = batchResult{klist: klines, err: err}
		}(i, batch)
	}
	wg.Wait()

	// 合并结果
	result := make(map[string][]*model.StockKlineWeekly)
	for _, br := range results {
		if br.err != nil {
			return nil, br.err
		}
		for _, k := range br.klist {
			list := result[k.Code]
			if len(list) < limit {
				result[k.Code] = append(list, k)
			}
		}
	}

	// 反转每个 code 的数据为日期升序
	for code, list := range result {
		for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
			list[i], list[j] = list[j], list[i]
		}
		result[code] = list
	}

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
