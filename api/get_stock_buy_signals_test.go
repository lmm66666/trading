package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"trading/business"
)

func TestGetStockBuySignalsMissingStrategy(t *testing.T) {
	r := setupTestRouter(
		&mockStockDataService{},
		&mockFinancialReportService{},
		&mockScheduler{},
		&mockSignalService{},
		&mockQueryService{},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/stocks/signal", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStockBuySignalsSuccess(t *testing.T) {
	r := setupTestRouter(
		&mockStockDataService{},
		&mockFinancialReportService{},
		&mockScheduler{},
		&mockSignalService{
			signal: &business.StrategySignal{Name: "weekly_b1_buy", Codes: []string{"600312", "600519"}},
		},
		&mockQueryService{},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/stocks/signal?strategy=weekly_b1_buy", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "600312") || !contains(body, "600519") {
		t.Fatalf("expected codes in response, got %s", body)
	}
	if contains(body, "short_detail") || contains(body, "long_detail") {
		t.Fatalf("response should not contain scoring details, got %s", body)
	}
}

func TestGetStockBuySignalsEmpty(t *testing.T) {
	r := setupTestRouter(
		&mockStockDataService{},
		&mockFinancialReportService{},
		&mockScheduler{},
		&mockSignalService{},
		&mockQueryService{},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/stocks/signal?strategy=daily_b1_buy", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetStockBuySignalsServiceError(t *testing.T) {
	r := setupTestRouter(
		&mockStockDataService{},
		&mockFinancialReportService{},
		&mockScheduler{},
		&mockSignalService{signalErr: errors.New("boom")},
		&mockQueryService{},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/stocks/signal?strategy=daily_b1_buy", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
