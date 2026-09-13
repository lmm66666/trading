package business

import (
	"context"

	"trading/model"
)

type mockBroker struct {
	financialData []*model.FinancialReport
	financialErr  error
}

func (*mockBroker) GetStockTodayInBatch(context.Context, []string) (map[string]*model.StockKline, error) {
	return nil, nil
}
func (*mockBroker) GetStockToday(context.Context, string) (*model.StockKline, error) { return nil, nil }
func (*mockBroker) GetStockHistorical(context.Context, string, int, int) ([]model.StockKline, error) {
	return nil, nil
}
func (m *mockBroker) GetFinancialReportHistorical(context.Context, string, int, int) ([]*model.FinancialReport, int, error) {
	return m.financialData, len(m.financialData), m.financialErr
}

type mockFinancialRepo struct {
	upErr    error
	reports  []*model.FinancialReport
	upserted []*model.FinancialReport
	codes    []string
	codesErr error
}

func (m *mockFinancialRepo) Upsert(_ context.Context, reports []*model.FinancialReport) error {
	m.upserted = reports
	return m.upErr
}
func (m *mockFinancialRepo) FindByCode(context.Context, string) ([]*model.FinancialReport, error) {
	return m.reports, nil
}
func (*mockFinancialRepo) FindByCodeWithPagination(context.Context, string, int, int) ([]*model.FinancialReport, error) {
	return nil, nil
}
func (m *mockFinancialRepo) FindAllCodes(context.Context) ([]string, error) {
	return m.codes, m.codesErr
}
