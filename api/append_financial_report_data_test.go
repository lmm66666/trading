package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"trading/business"
)

// mockFinancialSchedulerWithErr 支持控制 TriggerNow 返回错误的 mock
type mockFinancialSchedulerWithErr struct {
	triggerErr     error
	alreadyRunning bool
}

func (m *mockFinancialSchedulerWithErr) Start(ctx context.Context) {}
func (m *mockFinancialSchedulerWithErr) Stop()                     {}
func (m *mockFinancialSchedulerWithErr) TriggerNow(ctx context.Context) error {
	if m.alreadyRunning {
		return business.ErrSchedulerBusy
	}
	return m.triggerErr
}

func TestAppendFinancialReportDataSuccess(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockFinancialReportService{}, &mockFinancialSchedulerWithErr{}, nil, nil, nil)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report/append", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppendFinancialReportDataAlreadyRunning(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockFinancialReportService{}, &mockFinancialSchedulerWithErr{alreadyRunning: true}, nil, nil, nil)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report/append", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestAppendFinancialReportDataError(t *testing.T) {
	r := gin.New()
	h := NewStockHandler(&mockFinancialReportService{}, &mockFinancialSchedulerWithErr{triggerErr: errors.New("scheduler error")}, nil, nil, nil)
	r.POST("/api/stocks/financial-report/append", h.AppendFinancialReportData)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report/append", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
