package business

import (
	"context"
	"errors"
	"testing"
)

// TestSignalServiceFindBuySignalsByStrategyUnknown 未知策略名称返回错误
func TestSignalServiceFindBuySignalsByStrategyUnknown(t *testing.T) {
	svc := NewSignalService(&mockDailyRepo{}, &mockWeeklyRepo{}, &mockFinancialRepo{})

	result, err := svc.FindBuySignalsByStrategy(context.Background(), "unknown_strategy")
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// TestSignalServiceFindBuySignalsFindAllCodesError FindAllCodes 失败返回错误
func TestSignalServiceFindBuySignalsFindAllCodesError(t *testing.T) {
	dailyRepo := &mockDailyRepo{codesErr: errors.New("db error")}
	weeklyRepo := &mockWeeklyRepo{}

	svc := NewSignalService(dailyRepo, weeklyRepo, &mockFinancialRepo{})

	// FindBuySignals 扫描 daily 时 FindAllCodes 失败
	_, err := svc.FindBuySignals(context.Background())
	if err == nil {
		t.Fatal("expected error when FindAllCodes fails, got nil")
	}
}
