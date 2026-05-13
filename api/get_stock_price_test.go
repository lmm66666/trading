package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/model"
)

func TestGetStockPriceSuccess(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		prices: []*model.StockKlineDaily{
			{Code: "000001", Date: "2025-04-25", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/price", h.GetStockPrice)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/price?code=000001&cycle=daily", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStockPriceMissingCode(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, nil)
	r.GET("/api/stocks/price", h.GetStockPrice)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/price", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStockPriceInvalidCycle(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, nil)
	r.GET("/api/stocks/price", h.GetStockPrice)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/price?code=000001&cycle=monthly", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStockPriceServiceError(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{pricesErr: errors.New("db error")}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/price", h.GetStockPrice)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/price?code=000001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestGetStockPriceDefaultParams(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		prices: []*model.StockKlineDaily{
			{Code: "000001", Date: "2025-04-25", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/price", h.GetStockPrice)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/price?code=000001", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStockPricePagination(t *testing.T) {
	r := gin.New()
	querySvc := &mockQueryService{
		prices: []*model.StockKlineDaily{
			{Code: "000001", Date: "2025-04-25", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1000},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, querySvc, nil)
	r.GET("/api/stocks/price", h.GetStockPrice)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stocks/price?code=000001&pagesize=10&pagenum=2", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
