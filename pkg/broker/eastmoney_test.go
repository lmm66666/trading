package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"trading/model"
)

// TestEastMoneyBrokerImplementsInterface 验证 EastMoneyBroker 实现了 ShiborBroker 接口
func TestEastMoneyBrokerImplementsInterface(t *testing.T) {
	var _ ShiborBroker = (*EastMoneyBroker)(nil)
}

// TestFetchShibor 端到端测试获取 Shibor 数据
func TestFetchShibor(t *testing.T) {
	requireLiveBroker(t)
	b := NewEastMoneyBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	data, err := b.FetchShibor(ctx, "001")
	if err != nil {
		t.Fatalf("fetch shibor failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected shibor data, got none")
	}

	for _, d := range data {
		t.Logf("Shibor: date=%s period=%s rate=%.4f change=%.4f",
			d.ReportDate, d.ReportPeriod, d.IRRate, d.ChangeRate)
	}
}

// TestFetchShiborMockServer 使用 mock server 测试解析逻辑
func TestFetchShiborMockServer(t *testing.T) {
	mockData := []model.ShiborData{
		{ReportDate: "2026-05-12", ReportPeriod: "隔夜(O/N)", IRRate: 1.2380, ChangeRate: -3.30, IndicatorID: "001"},
		{ReportDate: "2026-05-11", ReportPeriod: "隔夜(O/N)", IRRate: 1.2710, ChangeRate: 2.10, IndicatorID: "001"},
	}
	resp := shiborResponse{Success: true}
	resp.Result.Data = mockData

	body, _ := json.Marshal(resp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	b := &EastMoneyBroker{
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: srv.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := b.FetchShibor(ctx, "001")
	if err != nil {
		t.Fatalf("fetch shibor from mock server failed: %v", err)
	}

	if len(data) != 2 {
		t.Fatalf("expected 2 records, got %d", len(data))
	}

	if data[0].IRRate != 1.2380 {
		t.Errorf("IRRate = %.4f, want 1.2380", data[0].IRRate)
	}
	if data[0].ChangeRate != -3.30 {
		t.Errorf("ChangeRate = %.4f, want -3.30", data[0].ChangeRate)
	}
	if data[0].ReportPeriod != "隔夜(O/N)" {
		t.Errorf("ReportPeriod = %s, want 隔夜(O/N)", data[0].ReportPeriod)
	}
}

// TestFetchShiborAPIError 测试 API 返回错误
func TestFetchShiborAPIError(t *testing.T) {
	resp := shiborResponse{Success: false}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	b := &EastMoneyBroker{
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: srv.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := b.FetchShibor(ctx, "001")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestAllShiborPeriods 验证预定义期限列表
func TestAllShiborPeriods(t *testing.T) {
	periods := AllShiborPeriods()
	if len(periods) != 8 {
		t.Fatalf("expected 8 periods, got %d", len(periods))
	}

	expected := map[string]string{
		"001": "SHIBOR_ON",
		"002": "SHIBOR_1W",
		"003": "SHIBOR_2W",
		"004": "SHIBOR_1M",
		"005": "SHIBOR_3M",
		"006": "SHIBOR_6M",
		"007": "SHIBOR_9M",
		"008": "SHIBOR_1Y",
	}

	for _, p := range periods {
		wantCode, ok := expected[p.ID]
		if !ok {
			t.Errorf("unexpected period ID: %s", p.ID)
			continue
		}
		if p.Code != wantCode {
			t.Errorf("period %s: Code = %s, want %s", p.ID, p.Code, wantCode)
		}
	}
}
