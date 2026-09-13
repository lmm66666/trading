package business

import (
	"context"

	"trading/data"
	"trading/model"
)

type QueryService interface {
	FindFinancialReportsByCode(context.Context, string, int, int) ([]*model.FinancialReport, error)
}

type queryService struct{ financialRepo data.FinancialReportRepo }

func NewQueryService(repo data.FinancialReportRepo) QueryService {
	return &queryService{financialRepo: repo}
}

func (s *queryService) FindFinancialReportsByCode(ctx context.Context, code string, limit, offset int) ([]*model.FinancialReport, error) {
	return s.financialRepo.FindByCodeWithPagination(ctx, code, limit, offset)
}
