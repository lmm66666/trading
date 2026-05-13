package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/model"
)

// mockMacroService 模拟宏观数据服务
type mockMacroService struct {
	shiborData    []model.ShiborData
	exchangeRates []*model.ExchangeRate
	err           error
}

func (m *mockMacroService) GetShibor(ctx context.Context, indicatorID string) ([]model.ShiborData, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.shiborData, nil
}

func (m *mockMacroService) GetExchangeRate(ctx context.Context, code string) ([]*model.ExchangeRate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.exchangeRates, nil
}

func TestGetShiborSuccess(t *testing.T) {
	r := gin.New()
	macroSvc := &mockMacroService{
		shiborData: []model.ShiborData{
			{ReportDate: "2026-05-12", ReportPeriod: "隔夜(O/N)", IRRate: 1.2380, ChangeRate: -3.30, IndicatorID: "001"},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, macroSvc)
	r.GET("/api/macro/shibor", h.GetShibor)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/macro/shibor?period=001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetShiborAllPeriods(t *testing.T) {
	r := gin.New()
	macroSvc := &mockMacroService{
		shiborData: []model.ShiborData{
			{ReportDate: "2026-05-12", ReportPeriod: "隔夜(O/N)", IRRate: 1.2380, IndicatorID: "001"},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, macroSvc)
	r.GET("/api/macro/shibor", h.GetShibor)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/macro/shibor", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetShiborServiceError(t *testing.T) {
	r := gin.New()
	macroSvc := &mockMacroService{err: errors.New("api error")}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, macroSvc)
	r.GET("/api/macro/shibor", h.GetShibor)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/macro/shibor?period=001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
