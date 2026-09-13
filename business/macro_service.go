package business

import (
	"context"
	"fmt"
	"log"

	"trading/model"
	"trading/pkg/broker"
)

// ExchangeRateBroker 汇率数据提供者接口
type ExchangeRateBroker interface {
	GetExchangeRate(ctx context.Context, code string) (*model.ExchangeRate, error)
	GetExchangeRateBatch(ctx context.Context, codes []string) (map[string]*model.ExchangeRate, error)
}

// MacroService 宏观数据查询服务
type MacroService interface {
	// GetShibor 获取 Shibor 利率数据
	// indicatorID: 001=隔夜, 002=1周... 为空则返回所有期限
	GetShibor(ctx context.Context, indicatorID string) ([]model.ShiborData, error)

	// GetExchangeRate 获取汇率实时数据
	// code: 汇率代码如 USDCNY，为空则返回所有预设汇率
	GetExchangeRate(ctx context.Context, code string) ([]*model.ExchangeRate, error)
}

type macroService struct {
	shiborBroker   broker.ShiborBroker
	exchangeBroker ExchangeRateBroker
	exchangeCodes  []string
}

// NewMacroService 创建 MacroService 实例
func NewMacroService(shiborBroker broker.ShiborBroker, exchangeBroker ExchangeRateBroker) MacroService {
	return &macroService{
		shiborBroker:   shiborBroker,
		exchangeBroker: exchangeBroker,
		exchangeCodes:  []string{"DINIW", "USDCNY", "USDJPY"},
	}
}

func (s *macroService) GetShibor(ctx context.Context, indicatorID string) ([]model.ShiborData, error) {
	if indicatorID != "" {
		return s.shiborBroker.FetchShibor(ctx, indicatorID)
	}

	periods := broker.AllShiborPeriods()
	result := make([]model.ShiborData, 0, len(periods)*20)
	for _, p := range periods {
		data, err := s.shiborBroker.FetchShibor(ctx, p.ID)
		if err != nil {
			log.Printf("fetch shibor %s failed: %v, skipping", p.Code, err)
			continue
		}
		result = append(result, data...)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("all shibor periods fetch failed")
	}
	return result, nil
}

func (s *macroService) GetExchangeRate(ctx context.Context, code string) ([]*model.ExchangeRate, error) {
	if code != "" {
		rate, err := s.exchangeBroker.GetExchangeRate(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("fetch exchange rate %s failed: %w", code, err)
		}
		return []*model.ExchangeRate{rate}, nil
	}

	result, err := s.exchangeBroker.GetExchangeRateBatch(ctx, s.exchangeCodes)
	if err != nil {
		return nil, fmt.Errorf("fetch exchange rate batch failed: %w", err)
	}

	rates := make([]*model.ExchangeRate, 0, len(result))
	for _, code := range s.exchangeCodes {
		if rate, ok := result[code]; ok {
			rates = append(rates, rate)
		}
	}
	return rates, nil
}
