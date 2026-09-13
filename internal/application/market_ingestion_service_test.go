package application

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
	"trading/internal/port"
	"trading/pkg/indicator"
)

var marketID = market.InstrumentID{Exchange: market.SSE, Code: "600000"}

func marketDate(day int) time.Time { return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC) }
func marketBar(tf market.Timeframe, day int) market.Bar {
	return market.Bar{Instrument: marketID, Timeframe: tf, OpenTime: marketDate(day), CloseTime: marketDate(day), Open: 100000, High: 110000, Low: 90000, Close: 100000, Volume: 100, Amount: 10000000}
}

type marketReadFake struct {
	port.MarketData
	latest               market.DataVersion
	latestErr, errorRead error
	stored               map[market.Timeframe][]market.Bar
	factors              []market.AdjustmentFactor
	ids                  []market.InstrumentID
	read                 func(market.InstrumentID, market.Timeframe, time.Time, time.Time, market.DataVersion)
}

func (f *marketReadFake) LatestCompleteVersion(context.Context) (market.DataVersion, error) {
	if f.latestErr != nil {
		return 0, f.latestErr
	}
	if f.latest == 0 {
		return 0, port.ErrMarketDataNotFound
	}
	return f.latest, nil
}
func (f *marketReadFake) Dataset(_ context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time, v market.DataVersion) (market.Dataset, []market.AdjustmentFactor, []market.CorporateAction, error) {
	if f.read != nil {
		f.read(id, tf, from, to, v)
	}
	if f.errorRead != nil {
		return market.Dataset{}, nil, nil, f.errorRead
	}
	var bars []market.Bar
	for _, b := range f.stored[tf] {
		if !b.CloseTime.Before(from) && !b.CloseTime.After(to) {
			b.Version = v
			bars = append(bars, b)
		}
	}
	d, err := market.NewDataset(id, tf, v, bars)
	factors := append([]market.AdjustmentFactor(nil), f.factors...)
	for i := range factors {
		factors[i].Version = v
	}
	return d, factors, nil, err
}
func (f *marketReadFake) Instruments(context.Context, port.InstrumentScope) ([]market.InstrumentID, error) {
	return append([]market.InstrumentID(nil), f.ids...), f.errorRead
}

type marketSourceFake struct {
	bars      map[market.Timeframe][]market.Bar
	factors   map[market.Timeframe][]market.AdjustmentFactor
	actions   []market.CorporateAction
	fail      market.Timeframe
	actionErr error
	fetch     func(context.Context, market.Timeframe, time.Time, time.Time) error
}

func (f *marketSourceFake) FetchBars(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error) {
	if f.fetch != nil {
		if err := f.fetch(ctx, tf, from, to); err != nil {
			return nil, nil, err
		}
	}
	if f.fail == tf {
		return nil, nil, port.ErrTemporary
	}
	var bars []market.Bar
	for _, b := range f.bars[tf] {
		if !b.CloseTime.Before(from) && !b.CloseTime.After(to) {
			bars = append(bars, b)
		}
	}
	return bars, append([]market.AdjustmentFactor(nil), f.factors[tf]...), nil
}
func (f *marketSourceFake) FetchCorporateActions(context.Context, market.InstrumentID) ([]market.CorporateAction, error) {
	return f.actions, f.actionErr
}

type marketWriterFake struct {
	batches []port.MarketWriteBatch
	err     error
}

func (f *marketWriterFake) Publish(ctx context.Context, b port.MarketWriteBatch) (market.DataVersion, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if f.err != nil {
		return 0, f.err
	}
	if _, _, err := port.CanonicalMarketBatch(b); err != nil {
		return 0, err
	}
	f.batches = append(f.batches, b)
	return market.DataVersion(len(f.batches)), nil
}
func ingestionFixture(t *testing.T) (*MarketIngestionService, *marketSourceFake, *marketReadFake, *marketWriterFake) {
	t.Helper()
	factor := market.AdjustmentFactor{EffectiveTime: marketDate(1), Numerator: 4, Denominator: 5}
	src := &marketSourceFake{bars: map[market.Timeframe][]market.Bar{market.Day: {marketBar(market.Day, 9)}, market.Week: {marketBar(market.Week, 9)}}, factors: map[market.Timeframe][]market.AdjustmentFactor{market.Day: {factor}, market.Week: {factor}}, actions: []market.CorporateAction{{ID: "dividend", Instrument: marketID, ExDate: marketDate(9), Kind: market.CashDividend, CashPerShare: 100}}}
	data := &marketReadFake{}
	writer := &marketWriterFake{}
	svc, err := NewMarketIngestionService(src, data, writer, MarketIngestionConfig{Source: "fixture", HistoryStart: marketDate(1), Clock: func() time.Time { return marketDate(31) }, Limiter: indicator.NewLimiter(2)})
	require.NoError(t, err)
	return svc, src, data, writer
}
func TestRefreshPublishesRawAdjustedAndActionsTogether(t *testing.T) {
	svc, _, _, writer := ingestionFixture(t)
	got, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Equal(t, port.DataComplete, got.Quality)
	require.EqualValues(t, 1, got.Version)
	require.Len(t, writer.batches, 1)
	require.Len(t, writer.batches[0].Bars[market.Day], 1)
	require.Len(t, writer.batches[0].Bars[market.Week], 1)
	require.Len(t, writer.batches[0].Factors, 1)
	require.Len(t, writer.batches[0].Digest, 64)
}
func TestRefreshDoesNotPublishPartialSourceResponse(t *testing.T) {
	for _, scenario := range []string{"daily", "weekly", "actions", "empty", "factor", "conflict", "duplicate", "invalid", "stored", "latest", "publish", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			svc, src, data, writer := ingestionFixture(t)
			ctx := context.Background()
			want := error(ErrIncompleteMarketData)
			switch scenario {
			case "daily":
				src.fail = market.Day
				want = port.ErrTemporary
			case "weekly":
				src.fail = market.Week
				want = port.ErrTemporary
			case "actions":
				src.actionErr = port.ErrTemporary
				want = port.ErrTemporary
			case "empty":
				src.bars[market.Week] = nil
			case "factor":
				src.factors[market.Day] = nil
			case "conflict":
				src.factors[market.Week][0].Numerator = 3
			case "duplicate":
				src.bars[market.Day] = append(src.bars[market.Day], src.bars[market.Day][0])
				want = port.ErrInvalidPortValue
			case "invalid":
				src.bars[market.Day][0].OpenTime = time.Time{}
				want = port.ErrInvalidPortValue
			case "stored":
				data.latest = 1
				data.errorRead = port.ErrTemporary
				want = port.ErrTemporary
			case "latest":
				data.latestErr = port.ErrTemporary
				want = port.ErrTemporary
			case "publish":
				writer.err = port.ErrTemporary
				want = port.ErrTemporary
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				want = context.Canceled
			}
			_, err := svc.Refresh(ctx, marketID)
			require.ErrorIs(t, err, want)
			require.Empty(t, writer.batches)
		})
	}
}
func TestRefreshRefetchesTwentyStoredTradingBarsAndRejectsLostDates(t *testing.T) {
	svc, src, data, writer := ingestionFixture(t)
	var days, weeks []market.Bar
	for day := 1; day <= 30; day++ {
		days = append(days, marketBar(market.Day, day))
		weeks = append(weeks, marketBar(market.Week, day))
	}
	data.latest = 7
	data.stored = map[market.Timeframe][]market.Bar{market.Day: days, market.Week: weeks}
	data.factors = src.factors[market.Day]
	src.bars = data.stored
	src.fetch = func(_ context.Context, tf market.Timeframe, from, to time.Time) error {
		require.Equal(t, marketDate(11), from)
		require.Equal(t, marketDate(31), to)
		return nil
	}
	_, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Len(t, writer.batches[0].Bars[market.Day], 20)
	src.bars = map[market.Timeframe][]market.Bar{market.Day: days[11:], market.Week: weeks}
	_, err = svc.Refresh(context.Background(), marketID)
	require.ErrorIs(t, err, ErrIncompleteMarketData)
	require.Len(t, writer.batches, 1)
}
func TestRefreshRejectsInvalidID(t *testing.T) {
	svc, _, _, _ := ingestionFixture(t)
	_, err := svc.Refresh(context.Background(), market.InstrumentID{})
	require.ErrorIs(t, err, ErrInvalidRequest)
}
func TestRefreshRejectsContradictoryDailyAndWeeklyClose(t *testing.T) {
	svc, src, _, writer := ingestionFixture(t)
	src.bars[market.Week][0].Close = 105000
	_, err := svc.Refresh(context.Background(), marketID)
	require.ErrorIs(t, err, ErrIncompleteMarketData)
	require.Empty(t, writer.batches)
}

func TestRefreshRebasesHistoryAndReplacesObsoleteFactorBreakpoints(t *testing.T) {
	svc, src, data, writer := ingestionFixture(t)
	var days, weeks []market.Bar
	for day := 1; day <= 30; day++ {
		days = append(days, marketBar(market.Day, day))
		weeks = append(weeks, marketBar(market.Week, day))
	}
	data.latest = 7
	data.stored = map[market.Timeframe][]market.Bar{market.Day: days, market.Week: weeks}
	data.factors = []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Numerator: 1, Denominator: 1}, {EffectiveTime: marketDate(20), Numerator: 1, Denominator: 2}}
	src.bars = data.stored
	calls := 0
	src.fetch = func(_ context.Context, tf market.Timeframe, from, to time.Time) error {
		calls++
		if calls > 2 {
			require.Equal(t, marketDate(1), from)
		}
		return nil
	}
	got, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Equal(t, 4, calls)
	require.Equal(t, 30, got.DailyBars)
	factors := writer.batches[0].Factors
	require.Contains(t, factors, market.AdjustmentFactor{EffectiveTime: marketDate(20), Numerator: 4, Denominator: 5})
}
func TestRefreshDetectsKnownCrossTimeframeGapsWithoutWeekdayCalendar(t *testing.T) {
	svc, src, data, writer := ingestionFixture(t)
	days := []market.Bar{marketBar(market.Day, 9), marketBar(market.Day, 16), marketBar(market.Day, 23), marketBar(market.Day, 30)}
	weeks := []market.Bar{marketBar(market.Week, 2), marketBar(market.Week, 9), marketBar(market.Week, 30)}
	data.latest = 1
	data.stored = map[market.Timeframe][]market.Bar{market.Day: days, market.Week: weeks}
	data.factors = src.factors[market.Day]
	src.bars = map[market.Timeframe][]market.Bar{market.Day: append([]market.Bar{marketBar(market.Day, 2)}, days...), market.Week: append(weeks, marketBar(market.Week, 16), marketBar(market.Week, 23))}
	src.fetch = func(_ context.Context, tf market.Timeframe, from, to time.Time) error {
		if tf == market.Day {
			require.Equal(t, marketDate(2), from)
		} else {
			require.Equal(t, marketDate(9), from)
		}
		return nil
	}
	_, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Len(t, writer.batches[0].Bars[market.Day], 5)
}
func TestRefreshGuardAndLimiterCancellation(t *testing.T) {
	svc, src, _, writer := ingestionFixture(t)
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src.fetch = func(ctx context.Context, _ market.Timeframe, _, _ time.Time) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() { _, err := svc.Refresh(ctx, marketID); done <- err }()
	<-entered
	_, err := svc.Refresh(context.Background(), marketID)
	require.ErrorIs(t, err, ErrRefreshAlreadyRunning)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Empty(t, writer.batches)
	limiter := indicator.NewLimiter(1)
	require.NoError(t, limiter.Acquire(context.Background()))
	defer limiter.Release()
	svc.config.Limiter = limiter
	timeout, stop := context.WithTimeout(context.Background(), time.Millisecond)
	defer stop()
	_, err = svc.Refresh(timeout, marketID)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
func TestRefreshConfigurationAndSourceBounds(t *testing.T) {
	svc, src, data, writer := ingestionFixture(t)
	for _, cfg := range []MarketIngestionConfig{{}, {Source: "valid"}, {Source: "valid", HistoryStart: marketDate(1).In(time.FixedZone("local", 3600))}} {
		_, err := NewMarketIngestionService(src, data, writer, cfg)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
	_, err := NewMarketIngestionService(nil, data, writer, svc.config)
	require.ErrorIs(t, err, ErrInvalidRequest)
	cfg := svc.config
	cfg.Clock = nil
	_, err = NewMarketIngestionService(src, data, writer, cfg)
	require.NoError(t, err)
	svc.config.HistoryStart = marketDate(32)
	_, err = svc.Refresh(context.Background(), marketID)
	require.ErrorIs(t, err, ErrInvalidRequest)
	svc.config.HistoryStart = marketDate(1)
	svc.config.Limiter = nil
	for _, mutate := range []func(){
		func() {
			src.factors[market.Day] = []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Numerator: 0, Denominator: 1}}
		},
		func() {
			src.factors[market.Day] = []market.AdjustmentFactor{{EffectiveTime: marketDate(1), Numerator: 1, Denominator: 1}, {EffectiveTime: marketDate(1), Numerator: 1, Denominator: 1}}
		},
		func() {
			src.factors[market.Day] = []market.AdjustmentFactor{{EffectiveTime: marketDate(32), Numerator: 1, Denominator: 1}}
		},
		func() {
			src.factors[market.Day] = []market.AdjustmentFactor{{EffectiveTime: time.Time{}, Numerator: 1, Denominator: 1}}
		},
		func() { src.bars[market.Day] = make([]market.Bar, 10001) },
	} {
		mutate()
		_, err = svc.Refresh(context.Background(), marketID)
		require.Error(t, err)
		require.Empty(t, writer.batches)
	}
}
