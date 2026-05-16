package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"trading/business"
	"trading/pkg/strategy"
)

func TestGetStockBacktest_MissingCode(t *testing.T) {
	signalSvc := &mockSignalService{}
	router := setupTestRouter(nil, nil, nil, signalSvc, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/backtest?strategy=daily_b1_buy", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStockBacktest_MissingStrategy(t *testing.T) {
	signalSvc := &mockSignalService{}
	router := setupTestRouter(nil, nil, nil, signalSvc, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/backtest?code=600150", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStockBacktest_Success(t *testing.T) {
	signalSvc := &mockSignalService{
		backtestResult: &business.BacktestResult{
			Code:     "600150",
			Strategy: "daily_b1_buy",
			Cycle:    "daily",
			Signals:  []strategy.Signal{{Date: "2026-01-15"}},
		},
	}
	router := setupTestRouter(nil, nil, nil, signalSvc, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/backtest?code=600150&strategy=daily_b1_buy", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data := resp["data"].(map[string]any)
	if data["code"] != "600150" {
		t.Fatalf("expected code 600150, got %v", data["code"])
	}
}

func TestGetStockBacktest_InternalError(t *testing.T) {
	signalSvc := &mockSignalService{
		backtestErr: errors.New("db error"),
	}
	router := setupTestRouter(nil, nil, nil, signalSvc, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/backtest?code=600150&strategy=daily_b1_buy", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
