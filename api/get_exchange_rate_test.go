package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/model"
)

func TestGetExchangeRateByCode(t *testing.T) {
	r := gin.New()
	macroSvc := &mockMacroService{
		exchangeRates: []*model.ExchangeRate{
			{Code: "USDCNY", Name: "美元/人民币", Open: 7.2000, Now: 7.2150, ChangePercent: 0.21},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, macroSvc)
	r.GET("/api/macro/exchange-rate", h.GetExchangeRate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/macro/exchange-rate?code=USDCNY", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetExchangeRateAll(t *testing.T) {
	r := gin.New()
	macroSvc := &mockMacroService{
		exchangeRates: []*model.ExchangeRate{
			{Code: "USDCNY", Name: "美元/人民币", Now: 7.2150},
			{Code: "USDJPY", Name: "美元/日元", Now: 148.50},
			{Code: "DINIW", Name: "美元指数", Now: 104.30},
		},
	}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, macroSvc)
	r.GET("/api/macro/exchange-rate", h.GetExchangeRate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/macro/exchange-rate", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetExchangeRateServiceError(t *testing.T) {
	r := gin.New()
	macroSvc := &mockMacroService{err: errors.New("api error")}
	h := NewStockHandler(&mockStockDataService{}, &mockFinancialReportService{}, &mockScheduler{}, &mockFinancialScheduler{}, nil, &mockQueryService{}, macroSvc)
	r.GET("/api/macro/exchange-rate", h.GetExchangeRate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/macro/exchange-rate?code=USDCNY", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
