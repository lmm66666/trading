package business

import (
	"context"
	"fmt"
	"sync"

	"trading/data"
	"trading/model"
	"trading/pkg/filter/financial"
	"trading/pkg/strategy"
)

// StrategySignal 单个策略的扫描结果
type StrategySignal struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
}

// BacktestResult 回测结果
type BacktestResult struct {
	Code     string            `json:"code"`
	Strategy string            `json:"strategy"`
	Cycle    string            `json:"cycle"`
	Signals  []strategy.Signal `json:"signals"`
}

// SignalService 信号扫描服务
type SignalService interface {
	// FindBuySignals 扫描所有股票，返回每个策略对应的买点股票列表
	FindBuySignals(ctx context.Context) ([]StrategySignal, error)
	// FindBuySignalsByStrategy 按策略名称扫描，只返回该策略的结果
	FindBuySignalsByStrategy(ctx context.Context, name string) (*StrategySignal, error)
	// FindFinancialReportSignals 扫描所有有财报数据的股票，返回满足财报策略的股票列表
	FindFinancialReportSignals(ctx context.Context, profitThreshold float64, quarterCount int) (*StrategySignal, error)
	// Backtest 对单只股票进行策略回测，返回历史上所有买入信号
	Backtest(ctx context.Context, code, strategyName, cycle string) (*BacktestResult, error)
}

type signalService struct {
	dailyRepo     data.StockKlineDailyRepo
	weeklyRepo    data.StockKlineWeeklyRepo
	financialRepo data.FinancialReportRepo
	stockInfo     StockInfoProvider
}

// NewSignalService 创建 SignalService 实例
func NewSignalService(dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo, financialRepo data.FinancialReportRepo, stockInfo StockInfoProvider) SignalService {
	return &signalService{dailyRepo: dailyRepo, weeklyRepo: weeklyRepo, financialRepo: financialRepo, stockInfo: stockInfo}
}

func (s *signalService) FindBuySignals(ctx context.Context) ([]StrategySignal, error) {
	var results []StrategySignal

	dailySigs, err := s.scanDailyStrategy(ctx, strategy.NewDailyB1BuyStrategy())
	if err != nil {
		return nil, err
	}
	if dailySigs != nil {
		results = append(results, *dailySigs)
	}

	weeklySigs, err := s.scanWeeklyStrategy(ctx, strategy.NewWeeklyB1BuyStrategy())
	if err != nil {
		return nil, err
	}
	if weeklySigs != nil {
		results = append(results, *weeklySigs)
	}

	return results, nil
}

func (s *signalService) FindBuySignalsByStrategy(ctx context.Context, name string) (*StrategySignal, error) {
	st, defaultCycle, err := createStrategy(name)
	if err != nil {
		return nil, err
	}
	if defaultCycle == "weekly" {
		return s.scanWeeklyStrategy(ctx, st)
	}
	return s.scanDailyStrategy(ctx, st)
}

// createStrategy 根据策略名称创建策略实例，返回策略、默认周期和错误
func createStrategy(name string) (*strategy.Strategy, string, error) {
	switch name {
	case "daily_b1_buy":
		return strategy.NewDailyB1BuyStrategy(), "daily", nil
	case "weekly_b1_buy":
		return strategy.NewWeeklyB1BuyStrategy(), "weekly", nil
	case "bottom_surge_pullback":
		return strategy.NewBottomSurgePullbackStrategy(), "daily", nil
	default:
		return nil, "", fmt.Errorf("unknown strategy: %s", name)
	}
}

func (s *signalService) Backtest(ctx context.Context, code, strategyName, cycle string) (*BacktestResult, error) {
	st, defaultCycle, err := createStrategy(strategyName)
	if err != nil {
		return nil, err
	}

	if cycle == "" {
		cycle = defaultCycle
	}

	switch cycle {
	case "daily":
		dailies, findErr := s.dailyRepo.FindByCode(ctx, code, 0)
		if findErr != nil {
			return nil, fmt.Errorf("find daily klines failed: %w", findErr)
		}
		if len(dailies) == 0 {
			return &BacktestResult{Code: code, Strategy: strategyName, Cycle: cycle}, nil
		}
		klines := dailyToKlines(dailies)
		sigs := st.ScanAll(klines)
		return &BacktestResult{Code: code, Strategy: strategyName, Cycle: cycle, Signals: sigs}, nil
	case "weekly":
		weeklies, findErr := s.weeklyRepo.FindByCode(ctx, code, 0)
		if findErr != nil {
			return nil, fmt.Errorf("find weekly klines failed: %w", findErr)
		}
		if len(weeklies) == 0 {
			return &BacktestResult{Code: code, Strategy: strategyName, Cycle: cycle}, nil
		}
		klines := weeklyToKlines(weeklies)
		fillDailyMA20(ctx, s.dailyRepo, code, klines)
		sigs := st.ScanAll(klines)
		return &BacktestResult{Code: code, Strategy: strategyName, Cycle: cycle, Signals: sigs}, nil
	default:
		return nil, fmt.Errorf("unsupported cycle: %s, must be daily or weekly", cycle)
	}
}

const (
	scanConcurrency = 50 // 并发扫描 goroutine 数（Phase3 后纯内存计算，可提高并发）
)

func (s *signalService) scanDailyStrategy(ctx context.Context, st *strategy.Strategy) (*StrategySignal, error) {
	codes, err := s.dailyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all daily codes failed: %w", err)
	}

	// 批量加载日线数据
	dailyMap, err := s.dailyRepo.FindRecentByCodes(ctx, codes, 70)
	if err != nil {
		return nil, fmt.Errorf("batch load daily klines failed: %w", err)
	}

	klinesMap := make(map[string][]*model.StockKline, len(dailyMap))
	for code, dailies := range dailyMap {
		klinesMap[code] = dailyToKlines(dailies)
	}

	return s.scanCodesFromMap(ctx, st, codes, klinesMap)
}

func (s *signalService) scanWeeklyStrategy(ctx context.Context, st *strategy.Strategy) (*StrategySignal, error) {
	codes, err := s.weeklyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all weekly codes failed: %w", err)
	}

	// 批量加载周线数据
	weeklyMap, err := s.weeklyRepo.FindRecentByCodes(ctx, codes, 70)
	if err != nil {
		return nil, fmt.Errorf("batch load weekly klines failed: %w", err)
	}

	// 批量加载日线数据用于填充 AuxMA20
	dailyMap, _ := s.dailyRepo.FindRecentByCodes(ctx, codes, dailyMALookback)

	klinesMap := make(map[string][]*model.StockKline, len(weeklyMap))
	for code, weeklies := range weeklyMap {
		klines := weeklyToKlines(weeklies)
		if dailies, ok := dailyMap[code]; ok {
			fillDailyMA20FromData(klines, dailies)
		}
		klinesMap[code] = klines
	}

	return s.scanCodesFromMap(ctx, st, codes, klinesMap)
}

// scanCodesFromMap 从预加载的 klines map 中扫描信号，纯内存计算无 IO
func (s *signalService) scanCodesFromMap(ctx context.Context, st *strategy.Strategy, codes []string, klinesMap map[string][]*model.StockKline) (*StrategySignal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var (
		matched []string
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	sem := make(chan struct{}, scanConcurrency)

	for _, code := range codes {
		if err := ctx.Err(); err != nil {
			wg.Wait()
			return nil, err
		}

		klines, ok := klinesMap[code]
		if !ok || len(klines) == 0 {
			continue
		}

		sem <- struct{}{}

		wg.Add(1)
		go func(c string, kl []*model.StockKline) {
			defer wg.Done()
			defer func() { <-sem }()

			sig := st.ScanLatest(kl)
			lastDate := kl[len(kl)-1].Date
			if sig != nil && sig.Date == lastDate {
				mu.Lock()
				matched = append(matched, c)
				mu.Unlock()
			}
		}(code, klines)
	}

	wg.Wait()

	if len(matched) == 0 {
		return nil, nil
	}
	return &StrategySignal{Name: st.Name(), Codes: matched}, nil
}

const dailyMALookback = 600

// fillDailyMA20 获取日线数据并计算日 20 均线，按日期映射到周线 klines 的 AuxMA20 字段
func fillDailyMA20(ctx context.Context, dailyRepo data.StockKlineDailyRepo, code string, klines []*model.StockKline) {
	if len(klines) == 0 {
		return
	}

	dailies, err := dailyRepo.FindByCode(ctx, code, dailyMALookback)
	if err != nil || len(dailies) == 0 {
		return
	}

	fillDailyMA20FromData(klines, dailies)
}

// fillDailyMA20FromData 使用预加载的日线数据填充 klines 的 AuxMA20 字段
func fillDailyMA20FromData(klines []*model.StockKline, dailies []*model.StockKlineDaily) {
	if len(klines) == 0 || len(dailies) == 0 {
		return
	}
	maMap := computeDailyMA20Map(dailies)
	for i := range klines {
		if v, ok := maMap[klines[i].Date]; ok {
			klines[i].AuxMA20 = v
		}
	}
}

func (s *signalService) FindFinancialReportSignals(ctx context.Context, profitThreshold float64, quarterCount int) (*StrategySignal, error) {
	st := strategy.NewFinancialStrategy("financial_profit_growth").
		AddFilter(financial.NewProfitGrowthFilter().WithThreshold(profitThreshold).WithQuarterCount(quarterCount))

	codes, err := s.financialRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all financial report codes failed: %w", err)
	}

	var matched []string
	for _, code := range codes {
		reports, findErr := s.financialRepo.FindByCode(ctx, code)
		if findErr != nil || len(reports) == 0 {
			continue
		}
		sig := st.Scan(reports)
		if sig != nil {
			matched = append(matched, code)
		}
	}

	if len(matched) == 0 {
		return nil, nil
	}
	return &StrategySignal{Name: st.Name(), Codes: matched}, nil
}
