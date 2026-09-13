package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"trading/business"
)

// TestGetFinancialReportSignalSuccess 正常返回财报信号
func TestGetFinancialReportSignalSuccess(t *testing.T) {
	signalSvc := &mockSignalService{
		signal: &business.StrategySignal{
			Name:  "financial_profit_growth",
			Codes: []string{"000001", "000002"},
		},
	}

	router := setupTestRouter(&mockFinancialReportService{}, signalSvc, &mockQueryService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report/signal", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Fatal("expected non-empty response body")
	}
}

// TestGetFinancialReportSignalWithParams 带参数调用
func TestGetFinancialReportSignalWithParams(t *testing.T) {
	signalSvc := &mockSignalService{
		signal: &business.StrategySignal{
			Name:  "financial_profit_growth",
			Codes: []string{"000001"},
		},
	}

	router := setupTestRouter(&mockFinancialReportService{}, signalSvc, &mockQueryService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report/signal?profit_threshold=0.15&quarter_count=3", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

// TestGetFinancialReportSignalInvalidThreshold 无效阈值返回 400
func TestGetFinancialReportSignalInvalidThreshold(t *testing.T) {
	router := setupTestRouter(&mockFinancialReportService{}, &mockSignalService{}, &mockQueryService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report/signal?profit_threshold=abc", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

// TestGetFinancialReportSignalServiceError 服务层错误返回 500
func TestGetFinancialReportSignalServiceError(t *testing.T) {
	signalSvc := &mockSignalService{signalErr: errors.New("db error")}
	router := setupTestRouter(&mockFinancialReportService{}, signalSvc, &mockQueryService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report/signal", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

// TestGetFinancialReportSignalEmptyResult 无匹配结果返回空列表
func TestGetFinancialReportSignalEmptyResult(t *testing.T) {
	signalSvc := &mockSignalService{signal: nil}
	router := setupTestRouter(&mockFinancialReportService{}, signalSvc, &mockQueryService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/financial-report/signal", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
