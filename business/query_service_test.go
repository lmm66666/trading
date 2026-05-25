package business

import (
	"context"
	"errors"
	"testing"

	"trading/model"
)

// mockQueryDailyRepo 用于 QueryService 测试的日线 mock
type mockQueryDailyRepo struct {
	findByCodeWithPaginationResult []*model.StockKlineDaily
	findByCodeWithPaginationErr    error
}

func (m *mockQueryDailyRepo) Create(ctx context.Context, kline *model.StockKlineDaily) error         { return nil }
func (m *mockQueryDailyRepo) CreateBatch(ctx context.Context, klines []*model.StockKlineDaily) error { return nil }
func (m *mockQueryDailyRepo) Upsert(ctx context.Context, klines []*model.StockKlineDaily) error      { return nil }
func (m *mockQueryDailyRepo) FindByID(ctx context.Context, id uint) (*model.StockKlineDaily, error)  { return nil, nil }
func (m *mockQueryDailyRepo) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockQueryDailyRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineDaily, error) {
	return m.findByCodeWithPaginationResult, m.findByCodeWithPaginationErr
}
func (m *mockQueryDailyRepo) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockQueryDailyRepo) FindAllCodes(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockQueryDailyRepo) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockQueryDailyRepo) Update(ctx context.Context, kline *model.StockKlineDaily) error {
	return nil
}
func (m *mockQueryDailyRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockQueryDailyRepo) List(ctx context.Context, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}

// mockQueryWeeklyRepo 用于 QueryService 测试的周线 mock
type mockQueryWeeklyRepo struct {
	findByCodeWithPaginationResult []*model.StockKlineWeekly
	findByCodeWithPaginationErr    error
}

func (m *mockQueryWeeklyRepo) Create(ctx context.Context, kline *model.StockKlineWeekly) error {
	return nil
}
func (m *mockQueryWeeklyRepo) CreateBatch(ctx context.Context, klines []*model.StockKlineWeekly) error {
	return nil
}
func (m *mockQueryWeeklyRepo) Upsert(ctx context.Context, klines []*model.StockKlineWeekly) error {
	return nil
}
func (m *mockQueryWeeklyRepo) FindByID(ctx context.Context, id uint) (*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockQueryWeeklyRepo) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockQueryWeeklyRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return m.findByCodeWithPaginationResult, m.findByCodeWithPaginationErr
}
func (m *mockQueryWeeklyRepo) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockQueryWeeklyRepo) FindAllCodes(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockQueryWeeklyRepo) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockQueryWeeklyRepo) Update(ctx context.Context, kline *model.StockKlineWeekly) error {
	return nil
}
func (m *mockQueryWeeklyRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockQueryWeeklyRepo) List(ctx context.Context, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}

// mockQueryFinancialRepo 用于 QueryService 测试的财报 mock
type mockQueryFinancialRepo struct {
	findByCodeWithPaginationResult []*model.FinancialReport
	findByCodeWithPaginationErr    error
}

func (m *mockQueryFinancialRepo) Upsert(ctx context.Context, reports []*model.FinancialReport) error {
	return nil
}
func (m *mockQueryFinancialRepo) FindByCode(ctx context.Context, code string) ([]*model.FinancialReport, error) {
	return nil, nil
}
func (m *mockQueryFinancialRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return m.findByCodeWithPaginationResult, m.findByCodeWithPaginationErr
}
func (m *mockQueryFinancialRepo) FindAllCodes(ctx context.Context) ([]string, error) { return nil, nil }

func TestQueryService_FindStockPricesByCode_Daily(t *testing.T) {
	dailyRepo := &mockQueryDailyRepo{
		findByCodeWithPaginationResult: []*model.StockKlineDaily{
			{Code: "sh600000", Date: "2024-01-01"},
		},
	}
	svc := NewQueryService(dailyRepo, &mockQueryWeeklyRepo{}, &mockQueryFinancialRepo{})

	result, err := svc.FindStockPricesByCode(context.Background(), "sh600000", "daily", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Code != "sh600000" {
		t.Errorf("expected code sh600000, got %s", result[0].Code)
	}
}

func TestQueryService_FindStockPricesByCode_Weekly(t *testing.T) {
	weeklyRepo := &mockQueryWeeklyRepo{
		findByCodeWithPaginationResult: []*model.StockKlineWeekly{
			{Code: "sh600000", Date: "2024-01-05"},
		},
	}
	svc := NewQueryService(&mockQueryDailyRepo{}, weeklyRepo, &mockQueryFinancialRepo{})

	result, err := svc.FindStockPricesByCode(context.Background(), "sh600000", "weekly", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Code != "sh600000" {
		t.Errorf("expected code sh600000, got %s", result[0].Code)
	}
}

func TestQueryService_FindStockPricesByCode_UnsupportedCycle(t *testing.T) {
	svc := NewQueryService(&mockQueryDailyRepo{}, &mockQueryWeeklyRepo{}, &mockQueryFinancialRepo{})

	_, err := svc.FindStockPricesByCode(context.Background(), "sh600000", "monthly", 10, 0)
	if err == nil {
		t.Fatal("expected error for unsupported cycle, got nil")
	}
}

func TestQueryService_FindStockPricesByCode_Error(t *testing.T) {
	dailyRepo := &mockQueryDailyRepo{
		findByCodeWithPaginationErr: errors.New("db error"),
	}
	svc := NewQueryService(dailyRepo, &mockQueryWeeklyRepo{}, &mockQueryFinancialRepo{})

	_, err := svc.FindStockPricesByCode(context.Background(), "sh600000", "daily", 10, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestQueryService_FindFinancialReportsByCode(t *testing.T) {
	financialRepo := &mockQueryFinancialRepo{
		findByCodeWithPaginationResult: []*model.FinancialReport{
			{Code: "sh600000"},
		},
	}
	svc := NewQueryService(&mockQueryDailyRepo{}, &mockQueryWeeklyRepo{}, financialRepo)

	result, err := svc.FindFinancialReportsByCode(context.Background(), "sh600000", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Code != "sh600000" {
		t.Errorf("expected code sh600000, got %s", result[0].Code)
	}
}

func TestQueryService_FindFinancialReportsByCode_Error(t *testing.T) {
	financialRepo := &mockQueryFinancialRepo{
		findByCodeWithPaginationErr: errors.New("db error"),
	}
	svc := NewQueryService(&mockQueryDailyRepo{}, &mockQueryWeeklyRepo{}, financialRepo)

	_, err := svc.FindFinancialReportsByCode(context.Background(), "sh600000", 10, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
