package business

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/model"
)

func TestQueryServiceRetainsOnlyFinancialReports(t *testing.T) {
	repo := &financialRepoStub{page: []*model.FinancialReport{{Code: "600000"}}}
	svc := NewQueryService(repo)
	got, err := svc.FindFinancialReportsByCode(context.Background(), "600000", 20, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
}
