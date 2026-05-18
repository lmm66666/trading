package business

import (
	"context"
	"fmt"

	"golang.org/x/time/rate"

	"trading/data"
	"trading/pkg/broker"
)

type StockDataService interface {
	SaveHistoricalData(ctx context.Context, code string) error
	AppendStockData(ctx context.Context, code string, daily, weekly bool) error
}

type stockDataService struct {
	broker     broker.Broker
	dailyRepo  data.StockKlineDailyRepo
	weeklyRepo data.StockKlineWeeklyRepo
	limiter    *rate.Limiter
}

// NewStockDataService 创建 StockDataService 实例
// limiter 为全局请求限流器（每次调 broker 前 Wait 一次）；传 nil 表示不限流，仅供测试使用
func NewStockDataService(b broker.Broker, dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo, limiter *rate.Limiter) StockDataService {
	return &stockDataService{broker: b, dailyRepo: dailyRepo, weeklyRepo: weeklyRepo, limiter: limiter}
}

// waitLimiter 调 broker 前的统一限流入口
func (s *stockDataService) waitLimiter(ctx context.Context) error {
	if s.limiter == nil {
		return nil
	}
	return s.limiter.Wait(ctx)
}

// SaveHistoricalData 从 broker 获取历史数据并保存到 DB
// 日线保存 1000 个点（scale=240），周线保存 200 个点（scale=1680）
// 周线最后一条若不是周五则丢弃，避免保存不完整周数据
func (s *stockDataService) SaveHistoricalData(ctx context.Context, code string) error {
	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	// 拉取日线
	if err := s.waitLimiter(ctx); err != nil {
		return fmt.Errorf("rate limiter wait failed: %w", err)
	}
	dailyKlines, err := s.broker.GetStockHistorical(ctx, symbol, 240, 1000)
	if err != nil {
		return fmt.Errorf("fetch daily historical failed: %w", err)
	}

	cleanedDaily := cleanKlines(dailyKlines)
	daily := toDaily(cleanedDaily)
	if err := s.dailyRepo.Upsert(ctx, daily); err != nil {
		return fmt.Errorf("upsert daily failed: %w", err)
	}

	// 拉取周线
	if err := s.waitLimiter(ctx); err != nil {
		return fmt.Errorf("rate limiter wait failed: %w", err)
	}
	weeklyKlines, err := s.broker.GetStockHistorical(ctx, symbol, 1680, 200)
	if err != nil {
		return fmt.Errorf("fetch weekly historical failed: %w", err)
	}

	cleanedWeekly := cleanKlines(weeklyKlines)
	filteredWeekly := filterIncompleteWeekly(cleanedWeekly)
	weekly := toWeekly(filteredWeekly)
	if err := s.weeklyRepo.Upsert(ctx, weekly); err != nil {
		return fmt.Errorf("upsert weekly failed: %w", err)
	}

	return nil
}

// AppendStockData 增量拉取并保存缺失的股票数据
// daily / weekly 分别控制是否拉取日线 / 周线；两者均为 false 时直接返回
func (s *stockDataService) AppendStockData(ctx context.Context, code string, daily, weekly bool) error {
	if !daily && !weekly {
		return nil
	}

	symbol, err := toSymbol(code)
	if err != nil {
		return err
	}

	if daily {
		if err := s.appendDaily(ctx, symbol, code); err != nil {
			return err
		}
	}
	if weekly {
		if err := s.appendWeekly(ctx, symbol, code); err != nil {
			return err
		}
	}
	return nil
}

func (s *stockDataService) appendDaily(ctx context.Context, symbol, code string) error {
	var lastDate string
	if latest, err := s.dailyRepo.FindLatestByCode(ctx, code); err == nil {
		lastDate = latest.Date
	}

	if err := s.waitLimiter(ctx); err != nil {
		return fmt.Errorf("rate limiter wait failed: %w", err)
	}
	klines, err := s.broker.GetStockHistorical(ctx, symbol, 240, 30)
	if err != nil {
		return fmt.Errorf("fetch daily failed: %w", err)
	}

	newKlines := filterAfterDate(cleanKlines(klines), lastDate)
	if len(newKlines) == 0 {
		return nil
	}

	if err := s.dailyRepo.Upsert(ctx, toDaily(newKlines)); err != nil {
		return fmt.Errorf("upsert daily failed: %w", err)
	}
	return nil
}

func (s *stockDataService) appendWeekly(ctx context.Context, symbol, code string) error {
	var lastDate string
	if latest, err := s.weeklyRepo.FindLatestByCode(ctx, code); err == nil {
		lastDate = latest.Date
	}

	if err := s.waitLimiter(ctx); err != nil {
		return fmt.Errorf("rate limiter wait failed: %w", err)
	}
	klines, err := s.broker.GetStockHistorical(ctx, symbol, 1680, 10)
	if err != nil {
		return fmt.Errorf("fetch weekly failed: %w", err)
	}

	newKlines := filterAfterDate(filterIncompleteWeekly(cleanKlines(klines)), lastDate)
	if len(newKlines) == 0 {
		return nil
	}

	if err := s.weeklyRepo.Upsert(ctx, toWeekly(newKlines)); err != nil {
		return fmt.Errorf("upsert weekly failed: %w", err)
	}
	return nil
}
