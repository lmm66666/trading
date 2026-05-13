package business

import (
	"context"
	"errors"
	"testing"

	"trading/model"
)

// mockShiborBroker 模拟 ShiborBroker
type mockShiborBroker struct {
	data       []model.ShiborData
	err        error
	periodErrs map[string]error
}

func (m *mockShiborBroker) FetchShibor(ctx context.Context, indicatorID string) ([]model.ShiborData, error) {
	if m.err != nil {
		return nil, m.err
	}
	if periodErr, ok := m.periodErrs[indicatorID]; ok {
		return nil, periodErr
	}
	return m.data, nil
}

// mockExchangeRateBroker 模拟汇率获取
type mockExchangeRateBroker struct {
	rates map[string]*model.ExchangeRate
	err   error
}

func (m *mockExchangeRateBroker) GetExchangeRate(ctx context.Context, code string) (*model.ExchangeRate, error) {
	if m.err != nil {
		return nil, m.err
	}
	rate, ok := m.rates[code]
	if !ok {
		return nil, errors.New("not found")
	}
	return rate, nil
}

func (m *mockExchangeRateBroker) GetExchangeRateBatch(ctx context.Context, codes []string) (map[string]*model.ExchangeRate, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make(map[string]*model.ExchangeRate)
	for _, code := range codes {
		if rate, ok := m.rates[code]; ok {
			result[code] = rate
		}
	}
	return result, nil
}

func TestMacroServiceGetShiborByPeriod(t *testing.T) {
	shiborBroker := &mockShiborBroker{
		data: []model.ShiborData{
			{ReportDate: "2026-05-12", ReportPeriod: "隔夜(O/N)", IRRate: 1.2380, ChangeRate: -3.30, IndicatorID: "001"},
		},
	}
	exchangeBroker := &mockExchangeRateBroker{}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	data, err := svc.GetShibor(context.Background(), "001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 record, got %d", len(data))
	}
	if data[0].IRRate != 1.2380 {
		t.Errorf("IRRate = %.4f, want 1.2380", data[0].IRRate)
	}
}

func TestMacroServiceGetShiborAllPeriods(t *testing.T) {
	shiborBroker := &mockShiborBroker{
		data: []model.ShiborData{
			{ReportDate: "2026-05-12", ReportPeriod: "隔夜(O/N)", IRRate: 1.2380, IndicatorID: "001"},
		},
	}
	exchangeBroker := &mockExchangeRateBroker{}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	data, err := svc.GetShibor(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 8 个期限，每个返回 1 条
	if len(data) != 8 {
		t.Fatalf("expected 8 records, got %d", len(data))
	}
}

func TestMacroServiceGetShiborError(t *testing.T) {
	shiborBroker := &mockShiborBroker{err: errors.New("network error")}
	exchangeBroker := &mockExchangeRateBroker{}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	_, err := svc.GetShibor(context.Background(), "001")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMacroServiceGetShiborPartialFailure(t *testing.T) {
	shiborBroker := &mockShiborBroker{
		data: []model.ShiborData{
			{ReportDate: "2026-05-12", ReportPeriod: "隔夜(O/N)", IRRate: 1.2380, IndicatorID: "001"},
		},
		periodErrs: map[string]error{
			"003": errors.New("timeout"), // 2周期限失败
			"007": errors.New("timeout"), // 9月期限失败
		},
	}
	exchangeBroker := &mockExchangeRateBroker{}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	data, err := svc.GetShibor(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 8 个期限中 2 个失败，每个成功期限返回 1 条
	if len(data) != 6 {
		t.Fatalf("expected 6 records, got %d", len(data))
	}
}

func TestMacroServiceGetShiborAllPeriodsFailed(t *testing.T) {
	shiborBroker := &mockShiborBroker{
		periodErrs: map[string]error{
			"001": errors.New("timeout"),
			"002": errors.New("timeout"),
			"003": errors.New("timeout"),
			"004": errors.New("timeout"),
			"005": errors.New("timeout"),
			"006": errors.New("timeout"),
			"007": errors.New("timeout"),
			"008": errors.New("timeout"),
		},
	}
	exchangeBroker := &mockExchangeRateBroker{}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	_, err := svc.GetShibor(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when all periods fail")
	}
}

func TestMacroServiceGetExchangeRateByCode(t *testing.T) {
	shiborBroker := &mockShiborBroker{}
	exchangeBroker := &mockExchangeRateBroker{
		rates: map[string]*model.ExchangeRate{
			"USDCNY": {Code: "USDCNY", Name: "美元/人民币", Open: 7.2000, Now: 7.2150, ChangePercent: 0.21},
		},
	}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	data, err := svc.GetExchangeRate(context.Background(), "USDCNY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 record, got %d", len(data))
	}
	if data[0].Now != 7.2150 {
		t.Errorf("Now = %.4f, want 7.2150", data[0].Now)
	}
}

func TestMacroServiceGetExchangeRateAll(t *testing.T) {
	shiborBroker := &mockShiborBroker{}
	exchangeBroker := &mockExchangeRateBroker{
		rates: map[string]*model.ExchangeRate{
			"USDCNY": {Code: "USDCNY", Name: "美元/人民币", Now: 7.2150},
			"USDJPY": {Code: "USDJPY", Name: "美元/日元", Now: 148.50},
			"DINIW":  {Code: "DINIW", Name: "美元指数", Now: 104.30},
		},
	}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	data, err := svc.GetExchangeRate(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 3 个预设汇率
	if len(data) != 3 {
		t.Fatalf("expected 3 records, got %d", len(data))
	}
}

func TestMacroServiceGetExchangeRateError(t *testing.T) {
	shiborBroker := &mockShiborBroker{}
	exchangeBroker := &mockExchangeRateBroker{err: errors.New("network error")}

	svc := NewMacroService(shiborBroker, exchangeBroker)
	_, err := svc.GetExchangeRate(context.Background(), "USDCNY")
	if err == nil {
		t.Fatal("expected error")
	}
}
