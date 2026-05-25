package business

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"trading/data"
	"trading/model"
	"trading/pkg/filter/financial"
	"trading/pkg/scorer"
	"trading/pkg/strategy"
)

// StrategySignal 单个策略的扫描结果
type StrategySignal struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
}

// ScoredSignal 带评分的信号股票
type ScoredSignal struct {
	Code        string           `json:"code"`
	ShortDetail *scorer.ScoreDetail `json:"short_detail"`
	LongDetail  *scorer.ScoreDetail `json:"long_detail,omitempty"`
}

// ScoredStrategySignal 带评分的策略扫描结果
type ScoredStrategySignal struct {
	Name    string          `json:"name"`
	Signals []ScoredSignal  `json:"signals"`
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
	// FindScoredSignalsByStrategy 按策略名称扫描，返回带评分的结果，按短线评分降序
	FindScoredSignalsByStrategy(ctx context.Context, name string) (*ScoredStrategySignal, error)
	// FindFinancialReportSignals 扫描所有有财报数据的股票，返回满足财报策略的股票列表
	FindFinancialReportSignals(ctx context.Context, profitThreshold float64, quarterCount int) (*StrategySignal, error)
	// Backtest 对单只股票进行策略回测，返回历史上所有买入信号
	Backtest(ctx context.Context, code, strategyName, cycle string) (*BacktestResult, error)
}

type signalService struct {
	dailyRepo     data.StockKlineDailyRepo
	weeklyRepo    data.StockKlineWeeklyRepo
	financialRepo data.FinancialReportRepo
}

// NewSignalService 创建 SignalService 实例
func NewSignalService(dailyRepo data.StockKlineDailyRepo, weeklyRepo data.StockKlineWeeklyRepo, financialRepo data.FinancialReportRepo) SignalService {
	return &signalService{dailyRepo: dailyRepo, weeklyRepo: weeklyRepo, financialRepo: financialRepo}
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

func (s *signalService) FindScoredSignalsByStrategy(ctx context.Context, name string) (*ScoredStrategySignal, error) {
	st, defaultCycle, err := createStrategy(name)
	if err != nil {
		return nil, err
	}

	// 1. 先扫描获取信号股票列表
	var signal *StrategySignal
	if defaultCycle == "weekly" {
		signal, err = s.scanWeeklyStrategy(ctx, st)
	} else {
		signal, err = s.scanDailyStrategy(ctx, st)
	}
	if err != nil {
		return nil, err
	}
	if signal == nil {
		return &ScoredStrategySignal{Name: name}, nil
	}

	// 2. 并发计算评分
	var (
		scored []ScoredSignal
		mu     sync.Mutex
		wg     sync.WaitGroup
	)
	sem := make(chan struct{}, scanConcurrency)

	for _, code := range signal.Codes {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			defer func() { <-sem }()

			ss := s.scoreOne(ctx, c, name, defaultCycle)
			mu.Lock()
			scored = append(scored, ss)
			mu.Unlock()
		}(code)
	}
	wg.Wait()

	// 3. 按短线评分降序
	sort.Slice(scored, func(i, j int) bool {
		si, sj := 0, 0
		if scored[i].ShortDetail != nil {
			si = scored[i].ShortDetail.Total
		}
		if scored[j].ShortDetail != nil {
			sj = scored[j].ShortDetail.Total
		}
		return si > sj
	})

	return &ScoredStrategySignal{Name: name, Signals: scored}, nil
}

func (s *signalService) scoreOne(ctx context.Context, code, strategyName, cycle string) ScoredSignal {
	ss := ScoredSignal{Code: code}

	switch cycle {
	case "daily":
		dailies, err := s.dailyRepo.FindByCode(ctx, code, 0)
		if err != nil || len(dailies) == 0 {
			return ss
		}
		klines := dailyToKlines(dailies)

		switch strategyName {
		case "bottom_surge_pullback":
			ss.ShortDetail = scorer.ScoreBottomSurgeShort(klines)
		default:
			ss.ShortDetail = scorer.ScoreBottomSurgeShort(klines)
		}

	case "weekly":
		weeklies, err := s.weeklyRepo.FindByCode(ctx, code, 0)
		if err != nil || len(weeklies) == 0 {
			return ss
		}
		weeklyKlines := weeklyToKlines(weeklies)
		fillDailyMA20(ctx, s.dailyRepo, code, weeklyKlines)

		// 获取日线数据用于跨周期评分
		dailies, _ := s.dailyRepo.FindByCode(ctx, code, 0)
		var dailyKlines []*model.StockKline
		if len(dailies) > 0 {
			dailyKlines = dailyToKlines(dailies)
		}

		switch strategyName {
		case "weekly_b1_buy":
			ss.ShortDetail = scorer.ScoreWeeklyB1Short(weeklyKlines, dailyKlines)
		default:
			ss.ShortDetail = scorer.ScoreWeeklyB1Short(weeklyKlines, dailyKlines)
		}
	}

	// 长线评分（财报）
	reports, err := s.financialRepo.FindByCode(ctx, code)
	if err == nil && len(reports) > 0 {
		ss.LongDetail = scorer.ScoreLongFinancial(reports)
	}

	return ss
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
	scanConcurrency = 20 // 并发扫描 goroutine 数
)

func (s *signalService) scanDailyStrategy(ctx context.Context, st *strategy.Strategy) (*StrategySignal, error) {
	codes, err := s.dailyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all daily codes failed: %w", err)
	}

	return s.scanCodes(ctx, st, codes, s.dailyRepo)
}

func (s *signalService) scanWeeklyStrategy(ctx context.Context, st *strategy.Strategy) (*StrategySignal, error) {
	codes, err := s.weeklyRepo.FindAllCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("find all weekly codes failed: %w", err)
	}

	return s.scanCodes(ctx, st, codes, s.weeklyRepo)
}

type klineRepo interface {
	FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineDaily, error)
}

type weeklyKlineRepo interface {
	FindByCode(ctx context.Context, code string, limit int) ([]*model.StockKlineWeekly, error)
}

func (s *signalService) scanCodes(ctx context.Context, st *strategy.Strategy, codes []string, repo any) (*StrategySignal, error) {
	var (
		matched []string
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	sem := make(chan struct{}, scanConcurrency)

	for _, code := range codes {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			defer func() { <-sem }()

			klines, sigs, lastDate := s.scanSingleCode(ctx, c, repo, st)
			if len(klines) == 0 {
				return
			}
			if len(sigs) > 0 && sigs[len(sigs)-1].Date == lastDate {
				mu.Lock()
				matched = append(matched, c)
				mu.Unlock()
			}
		}(code)
	}

	wg.Wait()

	if len(matched) == 0 {
		return nil, nil
	}
	return &StrategySignal{Name: st.Name(), Codes: matched}, nil
}

func (s *signalService) scanSingleCode(ctx context.Context, code string, repo any, st *strategy.Strategy) ([]*model.StockKline, []strategy.Signal, string) {
	switch r := repo.(type) {
	case data.StockKlineDailyRepo:
		dailies, err := r.FindByCode(ctx, code, 70)
		if err != nil || len(dailies) == 0 {
			return nil, nil, ""
		}
		lastDate := dailies[len(dailies)-1].Date
		klines := dailyToKlines(dailies)
		sigs := st.ScanAll(klines)
		return klines, sigs, lastDate
	case data.StockKlineWeeklyRepo:
		weeklies, err := r.FindByCode(ctx, code, 70)
		if err != nil || len(weeklies) == 0 {
			return nil, nil, ""
		}
		lastDate := weeklies[len(weeklies)-1].Date
		klines := weeklyToKlines(weeklies)
		// 填充跨周期日 20 日均线
		fillDailyMA20(ctx, s.dailyRepo, code, klines)
		sigs := st.ScanAll(klines)
		return klines, sigs, lastDate
	default:
		return nil, nil, ""
	}
}

// fillDailyMA20 获取日线数据并计算日 20 均线，按日期映射到周线 klines 的 AuxMA20 字段
func fillDailyMA20(ctx context.Context, dailyRepo data.StockKlineDailyRepo, code string, klines []*model.StockKline) {
	if len(klines) == 0 {
		return
	}

	dailies, err := dailyRepo.FindByCode(ctx, code, 0)
	if err != nil || len(dailies) == 0 {
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
