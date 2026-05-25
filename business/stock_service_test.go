package business

import (
	"context"
	"errors"
	"testing"

	"trading/model"
)

// mockBroker 模拟行情数据提供者
type mockBroker struct {
	dataByScale   map[int][]model.StockKline
	historicalErr error
	financialData []*model.FinancialReport
	financialErr  error
}

func (m *mockBroker) GetStockTodayInBatch(ctx context.Context, codes []string) (map[string]*model.StockKline, error) {
	return nil, nil
}

func (m *mockBroker) GetStockToday(ctx context.Context, code string) (*model.StockKline, error) {
	return nil, nil
}

func (m *mockBroker) GetStockHistorical(ctx context.Context, symbol string, scale int, length int) ([]model.StockKline, error) {
	if m.historicalErr != nil {
		return nil, m.historicalErr
	}
	return m.dataByScale[scale], nil
}

func (m *mockBroker) GetFinancialReportHistorical(ctx context.Context, symbol string, page, num int) ([]*model.FinancialReport, int, error) {
	if m.financialErr != nil {
		return nil, 0, m.financialErr
	}
	return m.financialData, 0, nil
}

// mockDailyRepo 模拟日线数据仓库
type mockDailyRepo struct {
	k        []*model.StockKlineDaily
	latest   *model.StockKlineDaily
	findErr  error
	upErr    error
	codesErr error
	codes    []string
}

func (m *mockDailyRepo) Create(ctx context.Context, kline *model.StockKlineDaily) error         { return nil }
func (m *mockDailyRepo) CreateBatch(ctx context.Context, klines []*model.StockKlineDaily) error { return nil }
func (m *mockDailyRepo) Upsert(ctx context.Context, klines []*model.StockKlineDaily) error      { return m.upErr }
func (m *mockDailyRepo) FindByID(ctx context.Context, id uint) (*model.StockKlineDaily, error)  { return nil, nil }
func (m *mockDailyRepo) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineDaily, error) {
	return m.k, m.findErr
}
func (m *mockDailyRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}
func (m *mockDailyRepo) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineDaily, error) {
	if m.latest == nil {
		return nil, errors.New("not found")
	}
	return m.latest, nil
}
func (m *mockDailyRepo) FindAllCodes(ctx context.Context) ([]string, error) {
	return m.codes, m.codesErr
}
func (m *mockDailyRepo) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineDaily, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	result := make(map[string][]*model.StockKlineDaily)
	for _, k := range m.k {
		result[k.Code] = append(result[k.Code], k)
	}
	return result, nil
}
func (m *mockDailyRepo) Update(ctx context.Context, kline *model.StockKlineDaily) error { return nil }
func (m *mockDailyRepo) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockDailyRepo) List(ctx context.Context, limit, offset int) ([]*model.StockKlineDaily, error) {
	return nil, nil
}

// mockFinancialRepo 模拟财报数据仓库
type mockFinancialRepo struct {
	upErr     error
	reports   []*model.FinancialReport // FindByCode 返回的已有数据
	upserted  []*model.FinancialReport // 记录 Upsert 实际接收的数据
	codes     []string
	codesErr  error
}

func (m *mockFinancialRepo) Upsert(ctx context.Context, reports []*model.FinancialReport) error {
	m.upserted = reports
	return m.upErr
}
func (m *mockFinancialRepo) FindByCode(ctx context.Context, code string) ([]*model.FinancialReport, error) {
	return m.reports, nil
}
func (m *mockFinancialRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return nil, nil
}
func (m *mockFinancialRepo) FindAllCodes(ctx context.Context) ([]string, error) {
	if m.codesErr != nil {
		return nil, m.codesErr
	}
	return m.codes, nil
}

// mockWeeklyRepo 模拟周线数据仓库
type mockWeeklyRepo struct {
	k        []*model.StockKlineWeekly
	latest   *model.StockKlineWeekly
	findErr  error
	upErr    error
	codesErr error
	codes    []string
}

func (m *mockWeeklyRepo) Create(ctx context.Context, kline *model.StockKlineWeekly) error         { return nil }
func (m *mockWeeklyRepo) CreateBatch(ctx context.Context, klines []*model.StockKlineWeekly) error { return nil }
func (m *mockWeeklyRepo) Upsert(ctx context.Context, klines []*model.StockKlineWeekly) error      { return m.upErr }
func (m *mockWeeklyRepo) FindByID(ctx context.Context, id uint) (*model.StockKlineWeekly, error)  { return nil, nil }
func (m *mockWeeklyRepo) FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error) {
	return m.k, m.findErr
}
func (m *mockWeeklyRepo) FindByCodeWithPagination(ctx context.Context, code string, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}
func (m *mockWeeklyRepo) FindLatestByCode(ctx context.Context, code string) (*model.StockKlineWeekly, error) {
	if m.latest == nil {
		return nil, errors.New("not found")
	}
	return m.latest, nil
}
func (m *mockWeeklyRepo) FindAllCodes(ctx context.Context) ([]string, error) {
	return m.codes, m.codesErr
}
func (m *mockWeeklyRepo) FindRecentByCodes(ctx context.Context, codes []string, limit int) (map[string][]*model.StockKlineWeekly, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	result := make(map[string][]*model.StockKlineWeekly)
	for _, k := range m.k {
		result[k.Code] = append(result[k.Code], k)
	}
	return result, nil
}
func (m *mockWeeklyRepo) Update(ctx context.Context, kline *model.StockKlineWeekly) error { return nil }
func (m *mockWeeklyRepo) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockWeeklyRepo) List(ctx context.Context, limit, offset int) ([]*model.StockKlineWeekly, error) {
	return nil, nil
}

// TestStockServiceSaveHistoricalDataSuccess 成功保存历史数据
func TestStockServiceSaveHistoricalDataSuccess(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240: {
				{Code: "sh000001", Date: "2025-04-20", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				{Code: "sh000001", Date: "2025-04-21", Open: 1.5, High: 3, Low: 1, Close: 2, Volume: 200},
			},
			1680: {
				{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				{Code: "sh000001", Date: "2025-04-25", Open: 1.5, High: 3, Low: 1, Close: 2, Volume: 200},
			},
		},
	}
	dailyRepo := &mockDailyRepo{}
	weeklyRepo := &mockWeeklyRepo{}

	svc := NewStockDataService(broker, dailyRepo, weeklyRepo, nil)
	err := svc.SaveHistoricalData(context.Background(), "000001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestStockServiceSaveHistoricalDataDropIncompleteWeekly 丢弃不完整周线
func TestStockServiceSaveHistoricalDataDropIncompleteWeekly(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240:  {{Code: "sh000001", Date: "2025-04-22", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
			1680: {
				{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				{Code: "sh000001", Date: "2025-04-22", Open: 1.5, High: 3, Low: 1, Close: 2, Volume: 200},
			},
		},
	}
	dailyRepo := &mockDailyRepo{}
	weeklyRepo := &mockWeeklyRepo{}

	svc := NewStockDataService(broker, dailyRepo, weeklyRepo, nil)
	err := svc.SaveHistoricalData(context.Background(), "000001")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestStockServiceSaveHistoricalDataInvalidCode 非法股票代码
func TestStockServiceSaveHistoricalDataInvalidCode(t *testing.T) {
	svc := NewStockDataService(&mockBroker{}, &mockDailyRepo{}, &mockWeeklyRepo{}, nil)
	err := svc.SaveHistoricalData(context.Background(), "999999")
	if err == nil {
		t.Fatal("expected error for invalid code, got nil")
	}
}

// TestStockServiceSaveHistoricalDataBrokerError broker 失败
func TestStockServiceSaveHistoricalDataBrokerError(t *testing.T) {
	broker := &mockBroker{historicalErr: errors.New("broker down")}
	svc := NewStockDataService(broker, &mockDailyRepo{}, &mockWeeklyRepo{}, nil)

	err := svc.SaveHistoricalData(context.Background(), "000001")
	if err == nil {
		t.Fatal("expected error when broker fails")
	}
}

// TestStockServiceSaveHistoricalDataDailyRepoError 日线 repo upsert 失败
func TestStockServiceSaveHistoricalDataDailyRepoError(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240:  {{Code: "sh000001", Date: "2025-04-20", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
			1680: {{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
		},
	}
	dailyRepo := &mockDailyRepo{upErr: errors.New("db down")}
	svc := NewStockDataService(broker, dailyRepo, &mockWeeklyRepo{}, nil)

	err := svc.SaveHistoricalData(context.Background(), "000001")
	if err == nil {
		t.Fatal("expected error when daily repo upsert fails")
	}
}

// TestStockServiceSaveHistoricalDataWeeklyRepoError 周线 repo upsert 失败
func TestStockServiceSaveHistoricalDataWeeklyRepoError(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240:  {{Code: "sh000001", Date: "2025-04-20", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
			1680: {{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
		},
	}
	weeklyRepo := &mockWeeklyRepo{upErr: errors.New("db down")}
	svc := NewStockDataService(broker, &mockDailyRepo{}, weeklyRepo, nil)

	err := svc.SaveHistoricalData(context.Background(), "000001")
	if err == nil {
		t.Fatal("expected error when weekly repo upsert fails")
	}
}

// TestAppendStockDataSuccess 增量保存成功，只保存新数据
func TestAppendStockDataSuccess(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240: {
				{Code: "sh000001", Date: "2025-04-20", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				{Code: "sh000001", Date: "2025-04-21", Open: 1.5, High: 3, Low: 1, Close: 2, Volume: 200},
			},
			1680: {
				{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				{Code: "sh000001", Date: "2025-04-25", Open: 1.5, High: 3, Low: 1, Close: 2, Volume: 200},
			},
		},
	}
	dailyRepo := &mockDailyRepo{latest: &model.StockKlineDaily{Code: "000001", Date: "2025-04-20"}}
	weeklyRepo := &mockWeeklyRepo{latest: &model.StockKlineWeekly{Code: "000001", Date: "2025-04-18"}}

	svc := NewStockDataService(broker, dailyRepo, weeklyRepo, nil)
	err := svc.AppendStockData(context.Background(), "000001", true, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestAppendStockDataEmptyDB 数据库为空时全量保存
func TestAppendStockDataEmptyDB(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240:  {{Code: "sh000001", Date: "2025-04-21", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
			1680: {{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
		},
	}
	dailyRepo := &mockDailyRepo{}
	weeklyRepo := &mockWeeklyRepo{}

	svc := NewStockDataService(broker, dailyRepo, weeklyRepo, nil)
	err := svc.AppendStockData(context.Background(), "000001", true, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestAppendStockDataNoNewData 没有新数据时不报错
func TestAppendStockDataNoNewData(t *testing.T) {
	broker := &mockBroker{
		dataByScale: map[int][]model.StockKline{
			240:  {{Code: "sh000001", Date: "2025-04-21", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
			1680: {{Code: "sh000001", Date: "2025-04-18", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}},
		},
	}
	dailyRepo := &mockDailyRepo{latest: &model.StockKlineDaily{Code: "000001", Date: "2025-04-21"}}
	weeklyRepo := &mockWeeklyRepo{latest: &model.StockKlineWeekly{Code: "000001", Date: "2025-04-18"}}

	svc := NewStockDataService(broker, dailyRepo, weeklyRepo, nil)
	err := svc.AppendStockData(context.Background(), "000001", true, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestAppendStockDataBrokerError broker 失败
func TestAppendStockDataBrokerError(t *testing.T) {
	broker := &mockBroker{historicalErr: errors.New("broker down")}
	svc := NewStockDataService(broker, &mockDailyRepo{}, &mockWeeklyRepo{}, nil)

	err := svc.AppendStockData(context.Background(), "000001", true, true)
	if err == nil {
		t.Fatal("expected error when broker fails")
	}
}
