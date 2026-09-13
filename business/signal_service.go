package business

import (
	"context"
	"fmt"
	"sort"

	"trading/data"
	"trading/internal/financialscreen"
)

type StrategySignal struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
}

type SignalService interface {
	FindFinancialReportSignals(context.Context, float64, int) (*StrategySignal, error)
}

type signalService struct{ financialRepo data.FinancialReportRepo }

func NewSignalService(repo data.FinancialReportRepo) SignalService {
	return &signalService{financialRepo: repo}
}

func (s *signalService) FindFinancialReportSignals(ctx context.Context, threshold float64, quarterCount int) (*StrategySignal, error) {
	filter, err := financialscreen.NewProfitGrowth(threshold, quarterCount)
	if err != nil {
		return nil, err
	}
	if s.financialRepo == nil {
		return nil, fmt.Errorf("financial repository is required")
	}
	codes, err := s.financialRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find financial report codes: %w", err)
	}
	matched := make([]string, 0)
	for _, code := range codes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		reports, err := s.financialRepo.FindByCode(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("find financial reports for %s: %w", code, err)
		}
		if filter.Match(reports) {
			matched = append(matched, code)
		}
	}
	if len(matched) == 0 {
		return nil, nil
	}
	sort.Strings(matched)
	return &StrategySignal{Name: "financial_profit_growth", Codes: matched}, nil
}
