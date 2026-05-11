package business

import (
	"context"
	"fmt"

	"trading/data"
	"trading/model"
	"trading/pkg/broker"
)

const defaultFinancialReportYears = 5
const defaultFinancialReportNum = defaultFinancialReportYears * 4

// FinancialReportService 财报数据服务接口
type FinancialReportService interface {
	SaveFinancialReportData(ctx context.Context, code string) error
	AppendFinancialReportData(ctx context.Context, code string) error
}

type financialReportService struct {
	broker        broker.Broker
	financialRepo data.FinancialReportRepo
}

// NewFinancialReportService 创建 FinancialReportService 实例
func NewFinancialReportService(b broker.Broker, financialRepo data.FinancialReportRepo) FinancialReportService {
	return &financialReportService{broker: b, financialRepo: financialRepo}
}

func (s *financialReportService) SaveFinancialReportData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	reports, _, err := s.broker.GetFinancialReportHistorical(ctx, symbol, 1, defaultFinancialReportNum)
	if err != nil {
		return fmt.Errorf("fetch financial report failed: %w", err)
	}

	if len(reports) == 0 {
		return nil
	}

	if err := s.financialRepo.Upsert(ctx, reports); err != nil {
		return fmt.Errorf("upsert financial report failed: %w", err)
	}

	return nil
}

func (s *financialReportService) AppendFinancialReportData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	existing, err := s.financialRepo.FindByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("find existing reports failed: %w", err)
	}

	existingDates := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		existingDates[r.ReportDate] = struct{}{}
	}

	reports, _, err := s.broker.GetFinancialReportHistorical(ctx, symbol, 1, 4)
	if err != nil {
		return fmt.Errorf("fetch financial report failed: %w", err)
	}

	var newReports []*model.FinancialReport
	for _, r := range reports {
		if _, ok := existingDates[r.ReportDate]; !ok {
			newReports = append(newReports, r)
		}
	}

	if len(newReports) == 0 {
		return nil
	}

	if err := s.financialRepo.Upsert(ctx, newReports); err != nil {
		return fmt.Errorf("upsert financial report failed: %w", err)
	}

	return nil
}
