package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSaveFinancialReportDataSuccess(t *testing.T) {
	r := setupTestRouter(&mockFinancialReportService{}, nil, nil)

	body, _ := json.Marshal(map[string]string{"code": "000001"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["code"].(float64) != 0 {
		t.Fatalf("expected code 0, got %v", resp["code"])
	}
}

func TestSaveFinancialReportDataMissingCode(t *testing.T) {
	r := setupTestRouter(&mockFinancialReportService{}, nil, nil)

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSaveFinancialReportDataServiceError(t *testing.T) {
	financialSvc := &mockFinancialReportService{saveErr: errors.New("service error")}
	r := setupTestRouter(financialSvc, nil, nil)

	body, _ := json.Marshal(map[string]string{"code": "000001"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/stocks/financial-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
