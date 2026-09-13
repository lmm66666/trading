package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"trading/internal/market"
	"trading/internal/port"
)

var (
	ErrIncompleteMarketData  = errors.New("incomplete market data")
	ErrRefreshAlreadyRunning = errors.New("market refresh already running")
)

// UpstreamLimiter 与已有 indicator.Limiter 兼容；成功获取后必须释放。
type UpstreamLimiter interface {
	Acquire(context.Context) error
	Release()
}
type MarketIngestionConfig struct {
	Source       string
	HistoryStart time.Time
	Clock        func() time.Time
	Limiter      UpstreamLimiter
}
type MarketIngestionService struct {
	source port.EquityDailySource
	data   port.MarketData
	writer port.MarketDataWriter
	config MarketIngestionConfig
}
type RefreshResult struct {
	Instrument market.InstrumentID `json:"instrument"`
	Version    market.DataVersion  `json:"version"`
	Quality    port.DataQuality    `json:"quality"`
	DailyBars  int                 `json:"daily_bars"`
	WeeklyBars int                 `json:"weekly_bars"`
}

// 全进程内同一证券只允许一个获取/发布事务，避免较慢请求回写旧观测。
var refreshInstruments = struct {
	sync.Mutex
	active map[market.InstrumentID]bool
}{active: map[market.InstrumentID]bool{}}

func NewMarketIngestionService(source port.EquityDailySource, data port.MarketData, writer port.MarketDataWriter, config MarketIngestionConfig) (*MarketIngestionService, error) {
	if source == nil || data == nil || writer == nil {
		return nil, invalidRequest("market dependencies are required")
	}
	if err := port.ValidateIdentity(config.Source, "source", port.MaxMarketSourceBytes, false); err != nil {
		return nil, invalidRequest("invalid source")
	}
	if err := validateUTCDate(config.HistoryStart, "history start"); err != nil {
		return nil, err
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &MarketIngestionService{source: source, data: data, writer: writer, config: config}, nil
}
func (s *MarketIngestionService) Refresh(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
	result := RefreshResult{Instrument: id, Quality: port.DataIncomplete}
	if err := id.Validate(); err != nil {
		return result, invalidRequest("invalid instrument")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	refreshInstruments.Lock()
	if refreshInstruments.active[id] {
		refreshInstruments.Unlock()
		return result, ErrRefreshAlreadyRunning
	}
	refreshInstruments.active[id] = true
	refreshInstruments.Unlock()
	defer func() { refreshInstruments.Lock(); delete(refreshInstruments.active, id); refreshInstruments.Unlock() }()
	to := s.config.Clock().UTC().Truncate(time.Microsecond)
	from := s.config.HistoryStart
	if to.Before(from) || to.After(from.AddDate(port.MaxBacktestRangeYears, 0, 0)) {
		return result, invalidRequest("invalid history window")
	}
	stored := map[market.Timeframe][]market.Bar{}
	oldFactors := map[market.Timeframe][]market.AdjustmentFactor{}
	version, err := s.data.LatestCompleteVersion(ctx)
	if err != nil && !errors.Is(err, port.ErrMarketDataNotFound) {
		return result, err
	}
	if err == nil {
		if version == 0 {
			return result, ErrIncompleteMarketData
		}
		ds, factors, _, err := s.data.Dataset(ctx, id, market.Day, from, to, version)
		if err != nil && !errors.Is(err, port.ErrMarketDataNotFound) {
			return result, err
		}
		if err == nil {
			if ds.Instrument() != id || ds.Timeframe() != market.Day || ds.Version() != version {
				return result, ErrIncompleteMarketData
			}
			stored[market.Day] = zeroVersionBars(ds.Bars())
			oldFactors[market.Day] = zeroVersionFactors(factors)
		}
	}
	start := dailyRefreshStart(from, stored[market.Day])
	batch, err := s.fetchBatch(ctx, id, start, to, stored[market.Day])
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	batch, err = replaceFactorBreakpoints(batch, oldFactors)
	if err != nil {
		return result, err
	}
	if err := validateFinalMarketBars(stored, batch.Bars); err != nil {
		return result, err
	}
	version, err = s.writer.Publish(ctx, batch)
	if err != nil {
		return result, err
	}
	if version == 0 {
		return result, ErrIncompleteMarketData
	}
	result.Version = version
	result.Quality = port.DataComplete
	result.DailyBars = len(batch.Bars[market.Day])
	result.WeeklyBars = len(batch.Bars[market.Week])
	return result, nil
}

// 按 writer 的增量 upsert 语义验证最终状态，包括本次窗口外保留的旧 Bar。
// 已观测周收盘证明对应日线应存在；后续已观测周收盘证明更早周组已经结束。
func validateFinalMarketBars(stored, updates map[market.Timeframe][]market.Bar) error {
	final := map[market.Timeframe]map[time.Time]market.Bar{
		market.Day: {}, market.Week: {},
	}
	for _, input := range []map[market.Timeframe][]market.Bar{stored, updates} {
		for _, tf := range []market.Timeframe{market.Day, market.Week} {
			for _, bar := range input[tf] {
				final[tf][bar.CloseTime] = bar
			}
		}
	}
	var latestWeeklyClose time.Time
	for at, weekly := range final[market.Week] {
		daily, ok := final[market.Day][at]
		if !ok || daily.Close != weekly.Close {
			return ErrIncompleteMarketData
		}
		if at.After(latestWeeklyClose) {
			latestWeeklyClose = at
		}
	}
	dailyWeekCloses := map[[2]int]time.Time{}
	var latestDailyClose time.Time
	for at := range final[market.Day] {
		year, week := at.ISOWeek()
		key := [2]int{year, week}
		if at.After(dailyWeekCloses[key]) {
			dailyWeekCloses[key] = at
		}
		if at.After(latestDailyClose) {
			latestDailyClose = at
		}
	}
	knownDailyWeeks := map[[2]int]bool{}
	for _, bar := range stored[market.Day] {
		year, week := bar.CloseTime.ISOWeek()
		knownDailyWeeks[[2]int{year, week}] = true
	}
	for key, at := range dailyWeekCloses {
		// 只有最终日线最新 ISO 周组可保持未知；后续日线周组本身证明更早周组已经结束。
		if at.Equal(latestDailyClose) && !knownDailyWeeks[key] && at.After(latestWeeklyClose) {
			continue
		}
		if _, ok := final[market.Week][at]; !ok {
			return ErrIncompleteMarketData
		}
	}
	return nil
}

func dailyRefreshStart(history time.Time, daily []market.Bar) time.Time {
	if len(daily) == 0 {
		return history
	}
	index := len(daily) - 20
	if index < 0 {
		index = 0
	}
	return daily[index].CloseTime
}
func (s *MarketIngestionService) fetchBatch(ctx context.Context, id market.InstrumentID, start, to time.Time, stored []market.Bar) (port.MarketWriteBatch, error) {
	batch := port.MarketWriteBatch{Source: s.config.Source, Instrument: id, Bars: map[market.Timeframe][]market.Bar{}}
	if err := s.acquire(ctx); err != nil {
		return batch, err
	}
	updates, err := s.source.FetchDailyBars(ctx, id, start, to)
	s.release()
	if err != nil {
		return batch, err
	}
	if len(updates) == 0 {
		return batch, ErrIncompleteMarketData
	}
	if len(updates) > port.MaxLookbackBars {
		return batch, fmt.Errorf("%w: source response too large", port.ErrInvalidPortValue)
	}
	observed := make(map[time.Time]bool, len(updates))
	for _, bar := range updates {
		if bar.Instrument != id || bar.Timeframe != market.Day || bar.CloseTime.Before(start) || bar.CloseTime.After(to) {
			return batch, ErrIncompleteMarketData
		}
		if observed[bar.CloseTime] {
			return batch, fmt.Errorf("%w: duplicate daily source bar", port.ErrInvalidPortValue)
		}
		observed[bar.CloseTime] = true
	}
	for _, bar := range stored {
		if !bar.CloseTime.Before(start) && !bar.CloseTime.After(to) && !observed[bar.CloseTime] {
			return batch, ErrIncompleteMarketData
		}
	}
	finalDaily, err := mergeDailyBars(id, stored, updates)
	if err != nil {
		return batch, err
	}
	weekly, err := market.AggregateWeekly(id, finalDaily)
	if err != nil {
		return batch, fmt.Errorf("%w: %v", port.ErrInvalidPortValue, err)
	}
	if err := s.acquire(ctx); err != nil {
		return batch, err
	}
	factors, err := s.source.FetchAdjustmentFactors(ctx, id)
	s.release()
	if err != nil {
		return batch, err
	}
	if len(factors) == 0 || len(factors) > port.MaxLookbackBars {
		return batch, ErrIncompleteMarketData
	}
	factors, err = normalizeFactors(factors)
	if err != nil {
		return batch, err
	}
	if factors[0].EffectiveTime.After(finalDaily[0].CloseTime) {
		first := factors[0]
		first.EffectiveTime = finalDaily[0].CloseTime
		factors = append([]market.AdjustmentFactor{first}, factors...)
	}
	for _, factor := range factors {
		if factor.EffectiveTime.After(to) {
			return batch, ErrIncompleteMarketData
		}
	}
	batch.Bars[market.Day] = finalDaily
	batch.Bars[market.Week] = weekly
	batch.Factors = factors
	batch, _, err = port.CanonicalMarketBatch(batch)
	if err != nil {
		return batch, fmt.Errorf("%w: %w", port.ErrInvalidPortValue, err)
	}
	for _, bars := range batch.Bars {
		factorIndex := 0
		for _, b := range bars {
			if _, ok := factorAt(batch.Factors, &factorIndex, b.CloseTime); !ok {
				return batch, ErrIncompleteMarketData
			}
		}
	}
	return batch, nil
}

func mergeDailyBars(id market.InstrumentID, stored, updates []market.Bar) ([]market.Bar, error) {
	byDate := make(map[time.Time]market.Bar, len(stored)+len(updates))
	for _, input := range [][]market.Bar{stored, updates} {
		for _, bar := range input {
			bar.Version = 0
			byDate[bar.CloseTime] = bar
		}
	}
	bars := make([]market.Bar, 0, len(byDate))
	for _, bar := range byDate {
		bars = append(bars, bar)
	}
	dataset, err := market.NewDataset(id, market.Day, 0, bars)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", port.ErrInvalidPortValue, err)
	}
	return dataset.Bars(), nil
}

func zeroVersionBars(input []market.Bar) []market.Bar {
	result := append([]market.Bar(nil), input...)
	for index := range result {
		result[index].Version = 0
	}
	return result
}

func zeroVersionFactors(input []market.AdjustmentFactor) []market.AdjustmentFactor {
	result := append([]market.AdjustmentFactor(nil), input...)
	for index := range result {
		result[index].Version = 0
	}
	return result
}
func (s *MarketIngestionService) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.config.Limiter != nil {
		return s.config.Limiter.Acquire(ctx)
	}
	return nil
}
func (s *MarketIngestionService) release() {
	if s.config.Limiter != nil {
		s.config.Limiter.Release()
	}
}
func normalizeFactors(input []market.AdjustmentFactor) ([]market.AdjustmentFactor, error) {
	factors := append([]market.AdjustmentFactor(nil), input...)
	sort.Slice(factors, func(i, j int) bool { return factors[i].EffectiveTime.Before(factors[j].EffectiveTime) })
	for i := range factors {
		f := &factors[i]
		if err := port.ValidateMarketTime(f.EffectiveTime); err != nil {
			return nil, err
		}
		if f.Numerator <= 0 || f.Denominator <= 0 {
			return nil, ErrIncompleteMarketData
		}
		if i > 0 && f.EffectiveTime.Equal(factors[i-1].EffectiveTime) {
			return nil, ErrIncompleteMarketData
		}
		a, b := f.Numerator, f.Denominator
		for b != 0 {
			a, b = b, a%b
		}
		f.Numerator /= a
		f.Denominator /= a
		f.Version = 0
	}
	return factors, nil
}
func factorAt(factors []market.AdjustmentFactor, index *int, at time.Time) (market.AdjustmentFactor, bool) {
	for *index < len(factors) && !factors[*index].EffectiveTime.After(at) {
		*index++
	}
	if *index == 0 {
		return market.AdjustmentFactor{}, false
	}
	return factors[*index-1], true
}
func equalFactor(a, b market.AdjustmentFactor) bool {
	return a.Numerator == b.Numerator && a.Denominator == b.Denominator
}

// 增量 upsert 不删除旧断点；用新时间线在其时点的值覆盖，避免被废弃断点继续生效。
func replaceFactorBreakpoints(batch port.MarketWriteBatch, previous map[market.Timeframe][]market.AdjustmentFactor) (port.MarketWriteBatch, error) {
	var from, to time.Time
	for _, bars := range batch.Bars {
		for _, b := range bars {
			if from.IsZero() || b.CloseTime.Before(from) {
				from = b.CloseTime
			}
			if b.CloseTime.After(to) {
				to = b.CloseTime
			}
		}
	}
	merged := map[time.Time]market.AdjustmentFactor{}
	for _, f := range batch.Factors {
		merged[f.EffectiveTime] = f
	}
	for _, old := range previous {
		factors, err := normalizeFactors(old)
		if err != nil {
			return batch, err
		}
		cursor := 0
		for _, f := range factors {
			if f.EffectiveTime.Before(from) || f.EffectiveTime.After(to) {
				continue
			}
			current, ok := factorAt(batch.Factors, &cursor, f.EffectiveTime)
			if !ok {
				return batch, ErrIncompleteMarketData
			}
			current.EffectiveTime = f.EffectiveTime
			merged[f.EffectiveTime] = current
		}
	}
	batch.Factors = nil
	for _, f := range merged {
		batch.Factors = append(batch.Factors, f)
	}
	batch.Digest = ""
	batch, _, err := port.CanonicalMarketBatch(batch)
	return batch, err
}
func factorsChanged(stored map[market.Timeframe][]market.Bar, previous map[market.Timeframe][]market.AdjustmentFactor, batch port.MarketWriteBatch) bool {
	for tf, bars := range batch.Bars {
		old, err := normalizeFactors(previous[tf])
		if err != nil {
			return true
		}
		oldDates := map[time.Time]bool{}
		for _, b := range stored[tf] {
			oldDates[b.CloseTime] = true
		}
		oldIndex, newIndex := 0, 0
		for _, b := range bars {
			a, ok := factorAt(old, &oldIndex, b.CloseTime)
			n, _ := factorAt(batch.Factors, &newIndex, b.CloseTime)
			if oldDates[b.CloseTime] && (!ok || !equalFactor(a, n)) {
				return true
			}
		}
	}
	return false
}
