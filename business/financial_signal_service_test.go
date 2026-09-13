package business

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/model"
)

type financialRepoStub struct {
	codes   []string
	reports map[string][]*model.FinancialReport
	page    []*model.FinancialReport
	err     error
}

func (r *financialRepoStub) Upsert(context.Context, []*model.FinancialReport) error { return nil }
func (r *financialRepoStub) FindByCode(_ context.Context, code string) ([]*model.FinancialReport, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.reports[code], nil
}
func (r *financialRepoStub) FindByCodeWithPagination(context.Context, string, int, int) ([]*model.FinancialReport, error) {
	return r.page, r.err
}
func (r *financialRepoStub) FindAllCodes(context.Context) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.codes, nil
}

func TestFinancialSignalServiceUsesRelocatedFilter(t *testing.T) {
	reports := []*model.FinancialReport{
		{ReportDate: "20230331", ReportType: 1, NetProfit: 100}, {ReportDate: "20230630", ReportType: 2, NetProfit: 250},
		{ReportDate: "20230930", ReportType: 3, NetProfit: 450}, {ReportDate: "20231231", ReportType: 4, NetProfit: 700},
		{ReportDate: "20240331", ReportType: 1, NetProfit: 120}, {ReportDate: "20240630", ReportType: 2, NetProfit: 300},
		{ReportDate: "20240930", ReportType: 3, NetProfit: 520}, {ReportDate: "20241231", ReportType: 4, NetProfit: 800},
	}
	svc := NewSignalService(&financialRepoStub{codes: []string{"600000"}, reports: map[string][]*model.FinancialReport{"600000": reports}})
	result, err := svc.FindFinancialReportSignals(context.Background(), 0.1, 4)
	require.NoError(t, err)
	require.Equal(t, []string{"600000"}, result.Codes)
}

func TestFinancialSignalServiceDoesNotSwallowRepositoryFailure(t *testing.T) {
	sentinel := errors.New("read failed")
	svc := NewSignalService(&financialRepoStub{codes: []string{"600000"}, err: sentinel})
	_, err := svc.FindFinancialReportSignals(context.Background(), 0.1, 4)
	require.ErrorIs(t, err, sentinel)
}
