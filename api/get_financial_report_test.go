package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/model"
)

func TestGetFinancialReportSuccess(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		reports: []*model.FinancialReport{
			{Code: "000001", ReportDate: "20251231", TotalRevenue: 1000, NetProfit: 100},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report?code=000001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFinancialReportMissingCode(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, nil)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetFinancialReportServiceError(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{reportsErr: errors.New("db error")}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report?code=000001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestGetFinancialReportPagination(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		reports: []*model.FinancialReport{
			{Code: "000001", ReportDate: "20251231", TotalRevenue: 1000, NetProfit: 100},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/financial-report", h.GetFinancialReport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report?code=000001&pagesize=5&pagenum=2", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
