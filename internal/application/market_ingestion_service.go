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
	source port.MarketSource
	data   port.MarketData
	writer port.MarketDataWriter
	config MarketIngestionConfig
}
type RefreshResult struct {
	Instrument            market.InstrumentID
	Version               market.DataVersion
	Quality               port.DataQuality
	DailyBars, WeeklyBars int
}

// 全进程内同一证券只允许一个获取/发布事务，避免较慢请求回写旧观测。
var refreshInstruments = struct {
	sync.Mutex
	active map[market.InstrumentID]bool
}{active: map[market.InstrumentID]bool{}}

func NewMarketIngestionService(source port.MarketSource, data port.MarketData, writer port.MarketDataWriter, config MarketIngestionConfig) (*MarketIngestionService, error) {
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
		for _, tf := range []market.Timeframe{market.Day, market.Week} {
			ds, factors, _, err := s.data.Dataset(ctx, id, tf, from, to, version)
			if errors.Is(err, port.ErrMarketDataNotFound) {
				continue
			}
			if err != nil {
				return result, err
			}
			if ds.Instrument() != id || ds.Timeframe() != tf || ds.Version() != version {
				return result, ErrIncompleteMarketData
			}
			stored[tf] = ds.Bars()
			oldFactors[tf] = factors
		}
	}
	starts := refreshStarts(from, stored)
	batch, err := s.fetchBatch(ctx, id, starts, to, stored)
	if err != nil {
		return result, err
	}
	// 前复权基准改变会影响窗口之前的历史。此时完整回补后再原子发布。
	if factorsChanged(stored, oldFactors, batch) && (starts[market.Day].After(from) || starts[market.Week].After(from)) {
		batch, err = s.fetchBatch(ctx, id, map[market.Timeframe]time.Time{market.Day: from, market.Week: from}, to, stored)
		if err != nil {
			return result, err
		}
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
	for at := range final[market.Day] {
		year, week := at.ISOWeek()
		key := [2]int{year, week}
		if at.After(dailyWeekCloses[key]) {
			dailyWeekCloses[key] = at
		}
	}
	knownDailyWeeks := map[[2]int]bool{}
	for _, bar := range stored[market.Day] {
		year, week := bar.CloseTime.ISOWeek()
		knownDailyWeeks[[2]int{year, week}] = true
	}
	for key, at := range dailyWeekCloses {
		// 已存日线发现的缺口必须补齐；仅新增且无已确认周收盘的末尾周组保留未知状态。
		if !knownDailyWeeks[key] && at.After(latestWeeklyClose) {
			continue
		}
		if _, ok := final[market.Week][at]; !ok {
			return ErrIncompleteMarketData
		}
	}
	return nil
}

func refreshStarts(history time.Time, stored map[market.Timeframe][]market.Bar) map[market.Timeframe]time.Time {
	starts := map[market.Timeframe]time.Time{market.Day: history, market.Week: history}
	daily, weekly := stored[market.Day], stored[market.Week]
	if len(daily) == 0 || len(weekly) == 0 {
		return starts
	}
	index := len(daily) - 20
	if index < 0 {
		index = 0
	}
	starts[market.Day] = daily[index].CloseTime
	starts[market.Week] = starts[market.Day]
	// 周期边界不靠自然日猜测：补抓覆盖窗口起点的已知周 Bar。
	for _, b := range weekly {
		if b.CloseTime.After(starts[market.Day]) {
			break
		}
		starts[market.Week] = b.CloseTime
	}
	dailyDates := map[time.Time]bool{}
	for _, b := range daily {
		dailyDates[b.CloseTime] = true
	}
	for _, b := range weekly {
		if !dailyDates[b.CloseTime] && b.CloseTime.Before(starts[market.Day]) {
			starts[market.Day] = b.CloseTime
		}
	}
	weeklyGroups := map[[2]int]bool{}
	for _, b := range weekly {
		y, w := b.CloseTime.ISOWeek()
		weeklyGroups[[2]int{y, w}] = true
	}
	for _, b := range daily {
		y, w := b.CloseTime.ISOWeek()
		if !weeklyGroups[[2]int{y, w}] && b.CloseTime.Before(starts[market.Week]) {
			starts[market.Week] = b.CloseTime
		}
	}
	return starts
}
func (s *MarketIngestionService) fetchBatch(ctx context.Context, id market.InstrumentID, starts map[market.Timeframe]time.Time, to time.Time, stored map[market.Timeframe][]market.Bar) (port.MarketWriteBatch, error) {
	batch := port.MarketWriteBatch{Source: s.config.Source, Instrument: id, Bars: map[market.Timeframe][]market.Bar{}}
	own := map[market.Timeframe][]market.AdjustmentFactor{}
	merged := map[time.Time]market.AdjustmentFactor{}
	for _, tf := range []market.Timeframe{market.Day, market.Week} {
		if err := s.acquire(ctx); err != nil {
			return batch, err
		}
		bars, factors, err := s.source.FetchBars(ctx, id, tf, starts[tf], to)
		s.release()
		if err != nil {
			return batch, err
		}
		if len(bars) == 0 {
			return batch, ErrIncompleteMarketData
		}
		if len(bars) > port.MaxLookbackBars || len(factors) > port.MaxLookbackBars {
			return batch, fmt.Errorf("%w: source response too large", port.ErrInvalidPortValue)
		}
		observed := map[time.Time]bool{}
		for _, b := range bars {
			if b.CloseTime.Before(starts[tf]) || b.CloseTime.After(to) {
				return batch, ErrIncompleteMarketData
			}
			observed[b.CloseTime] = true
		}
		for _, b := range stored[tf] {
			if !b.CloseTime.Before(starts[tf]) && !b.CloseTime.After(to) && !observed[b.CloseTime] {
				return batch, ErrIncompleteMarketData
			}
		}
		batch.Bars[tf] = bars
		normalized, err := normalizeFactors(factors)
		if err != nil {
			return batch, err
		}
		own[tf] = normalized
		for _, f := range normalized {
			if f.EffectiveTime.After(to) {
				return batch, ErrIncompleteMarketData
			}
			if previous, ok := merged[f.EffectiveTime]; ok && !equalFactor(previous, f) {
				return batch, ErrIncompleteMarketData
			}
			merged[f.EffectiveTime] = f
		}
	}
	dailyClose := make(map[time.Time]market.Price, len(batch.Bars[market.Day]))
	for _, bar := range batch.Bars[market.Day] {
		dailyClose[bar.CloseTime] = bar.Close
	}
	for _, bar := range batch.Bars[market.Week] {
		if close, ok := dailyClose[bar.CloseTime]; ok && close != bar.Close {
			return batch, ErrIncompleteMarketData
		}
	}
	if err := s.acquire(ctx); err != nil {
		return batch, err
	}
	actions, err := s.source.FetchCorporateActions(ctx, id)
	s.release()
	if err != nil {
		return batch, err
	}
	if len(actions) > port.MaxLookbackBars {
		return batch, fmt.Errorf("%w: too many corporate actions", port.ErrInvalidPortValue)
	}
	batch.Actions = actions
	for _, f := range merged {
		batch.Factors = append(batch.Factors, f)
	}
	batch, _, err = port.CanonicalMarketBatch(batch)
	if err != nil {
		return batch, fmt.Errorf("%w: %w", port.ErrInvalidPortValue, err)
	}
	for tf, bars := range batch.Bars {
		ownIndex, mergedIndex := 0, 0
		for _, b := range bars {
			a, aOK := factorAt(own[tf], &ownIndex, b.CloseTime)
			m, mOK := factorAt(batch.Factors, &mergedIndex, b.CloseTime)
			if !aOK || !mOK || !equalFactor(a, m) {
				return batch, ErrIncompleteMarketData
			}
		}
	}
	return batch, nil
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
