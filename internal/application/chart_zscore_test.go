package application

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

type pairData struct {
	port.MarketData
	bars     map[market.InstrumentID][]market.Bar
	err      error
	mismatch bool
	versions []market.DataVersion
}

func (f *pairData) LatestCompleteVersion(context.Context) (market.DataVersion, error) { return 7, nil }
func (f *pairData) Dataset(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time, v market.DataVersion) (market.Dataset, []market.AdjustmentFactor, []market.CorporateAction, error) {
	if err := (port.BatchRequest{PrimaryTimeframe: tf, From: from, To: to, Version: v}).Validate(); err != nil {
		return market.Dataset{}, nil, nil, err
	}
	f.versions = append(f.versions, v)
	if id != marketID && f.err != nil {
		return market.Dataset{}, nil, nil, f.err
	}
	bars := []market.Bar{}
	for _, b := range f.bars[id] {
		if !b.CloseTime.Before(from) && !b.CloseTime.After(to) {
			b.Timeframe = tf
			b.Version = v
			bars = append(bars, b)
		}
	}
	if f.mismatch && id != marketID {
		v++
		for i := range bars {
			bars[i].Version = v
		}
	}
	d, e := market.NewDataset(id, tf, v, bars)
	return d, []market.AdjustmentFactor{{EffectiveTime: from, Numerator: 1, Denominator: 2}}, nil, e
}
func newPair(t *testing.T) (*ChartQueryService, *pairData, ChartQuery) {
	other, err := market.ParseInstrumentID("SHFE:AU.MAIN")
	require.NoError(t, err)
	f := &pairData{bars: map[market.InstrumentID][]market.Bar{}}
	for _, id := range []market.InstrumentID{marketID, other} {
		for i := 0; i < 230; i++ {
			if id == other && i%3 != 0 {
				continue
			}
			at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
			price := market.Price((100 + i + i%7) * int(market.ValueScale))
			f.bars[id] = append(f.bars[id], market.Bar{Instrument: id, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: price, High: price, Low: price, Close: price, Version: 7})
		}
	}
	s, err := NewChartQueryService(f, chartCatalogFake{item: port.InstrumentSummary{ID: marketID, Active: true}})
	require.NoError(t, err)
	return s, f, ChartQuery{Instrument: marketID, Comparison: other.String(), Timeframe: market.Day, View: market.Raw, Limit: 1000, Indicators: []IndicatorRequest{{Kind: IndicatorZSCORE, Period: 5, Smooth: 3, Regime: 10}}}
}
func TestZScorePairPaginationAndAlignment(t *testing.T) {
	s, f, q := newPair(t)
	full, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, full.Series, 3)
	require.Len(t, full.ZScores, 1)
	require.Equal(t, []market.DataVersion{7, 7}, f.versions)
	require.Equal(t, f.bars[marketID][4].CloseTime, full.Series[0].Points[0].Time)
	// Commodity's last known bar is day 228, while the stock ends at 229.
	require.Equal(t, "2025-08-17", full.ZScores[0].CommodityDate)
	q.Limit = 100
	recent, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	q.DataVersion = recent.DataVersion
	q.Before = *recent.NextBefore
	older, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	for i := range full.Series {
		values := map[time.Time]float64{}
		for _, p := range full.Series[i].Points {
			values[p.Time] = p.Value
		}
		for _, page := range []ChartResult{recent, older} {
			require.Len(t, page.Series[i].Points, 100)
			for _, p := range page.Series[i].Points {
				require.InDelta(t, values[p.Time], p.Value, 1e-12)
			}
		}
	}
	q.Before = time.Time{}
	q.Indicators[0].Lag = 1
	lagged, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.NotEqual(t, recent.Series[0].Key, lagged.Series[0].Key)
	require.Equal(t, "2025-08-14", lagged.ZScores[0].CommodityDate)
	q.Indicators[0].Lag = 0
	q.View = market.ForwardAdjusted
	adjusted, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.InDelta(t, recent.Series[0].Points[0].Value, adjusted.Series[0].Points[0].Value, 1e-10)
}
func TestZScoreFailureIsolation(t *testing.T) {
	for _, mode := range []string{"missing", "weekly", "empty", "failure", "version", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, f, q := newPair(t)
			switch mode {
			case "missing":
				q.Comparison = ""
			case "weekly":
				q.Timeframe = market.Week
			case "empty":
				other, _ := market.ParseInstrumentID(q.Comparison)
				delete(f.bars, other)
			case "failure":
				f.err = errors.New("unavailable")
			case "version":
				f.mismatch = true
			case "cancel":
				f.err = context.Canceled
			}
			q.Indicators = append(q.Indicators, IndicatorRequest{Kind: IndicatorSMA, Period: 5})
			r, e := s.Query(context.Background(), q)
			if mode == "cancel" {
				require.ErrorIs(t, e, context.Canceled)
				return
			}
			require.NoError(t, e)
			require.NotEmpty(t, r.Bars)
			require.Len(t, r.Series, 1)
			require.NotEmpty(t, r.ZScores[0].Warning)
		})
	}
	for _, kind := range []IndicatorKind{"STD", "RETZ"} {
		s, _, q := newPair(t)
		q.Indicators = []IndicatorRequest{{Kind: kind, Period: 20}}
		_, e := s.Query(context.Background(), q)
		require.ErrorIs(t, e, ErrInvalidRequest)
	}
	s, _, q := newPair(t)
	q.Comparison = "INVALID"
	_, e := s.Query(context.Background(), q)
	require.ErrorIs(t, e, ErrInvalidRequest)
}

func TestZScoreStatesAndBounds(t *testing.T) {
	for _, tc := range []struct {
		z, long float64
		state   string
	}{{3, 3, "长期偏离，核查结构变化"}, {3, -3, "股票阶段性偏强"}, {-3, 3, "股票阶段性偏弱"}, {.5, 0, "常态区"}, {1.5, 0, "偏离观察"}} {
		require.Equal(t, tc.state, zScoreState(&tc.z, &tc.long))
	}
	require.Equal(t, "数据不足", zScoreState(nil, nil))
	for _, lag := range []int{-1, 6} {
		s, _, q := newPair(t)
		q.Indicators[0].Lag = lag
		_, err := s.Query(context.Background(), q)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
	s, f, q := newPair(t)
	q.Indicators = append(q.Indicators, IndicatorRequest{Kind: IndicatorSMA, Period: 5}, IndicatorRequest{Kind: IndicatorZSCORE, Period: 6, Smooth: 3, Regime: 10, Lag: 2})
	result, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, IndicatorSMA, result.Series[3].Kind)
	require.Len(t, f.versions, 2)
	// No prior commodity bar means no values, even if stock history exists.
	q.Before = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	q.Indicators = []IndicatorRequest{{Kind: IndicatorZSCORE, Period: 5, Smooth: 3, Regime: 10, Lag: 5}}
	result, err = s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, result.Series[0].Points)
	require.Empty(t, result.ZScores[0].CommodityDate)
}

func TestZScorePaginationKeepsTwentyYearOrigin(t *testing.T) {
	s, f, q := newPair(t)
	s.clock = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	origin := time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC)
	for id, bars := range f.bars {
		for i := range bars {
			bars[i].CloseTime = bars[i].CloseTime.AddDate(-19, 0, -10)
			bars[i].OpenTime = bars[i].CloseTime
		}
		f.bars[id] = bars
	}
	q.Indicators[0].Smooth = 500
	full, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, origin, full.Bars[0].CloseTime)
	q.Limit = 100
	recent, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	q.Before = *recent.NextBefore
	q.DataVersion = recent.DataVersion
	older, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	expected := map[time.Time]float64{}
	for _, p := range full.Series[1].Points {
		expected[p.Time] = p.Value
	}
	for _, p := range older.Series[1].Points {
		require.InDelta(t, expected[p.Time], p.Value, 1e-12)
	}
	q.Before = origin.Add(-12 * time.Hour)
	empty, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, empty.Bars)
	require.False(t, empty.HasMore)
}

func TestChartQueryWindowRespectsPortBounds(t *testing.T) {
	for _, at := range []time.Time{time.Date(2026, 9, 21, 14, 37, 12, 123000, time.UTC), time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)} {
		for _, cursor := range []time.Time{{}, at.AddDate(0, 0, 1), at.AddDate(0, -1, 0)} {
			s, _, q := newPair(t)
			s.clock = func() time.Time { return at }
			q.Before = cursor
			q.Comparison = ""
			_, err := s.Query(context.Background(), q)
			require.NoError(t, err)
		}
	}
}

func TestZScoreMissingCommodityIsNotRetryableReadFailure(t *testing.T) {
	s, f, q := newPair(t)
	f.err = port.ErrMarketDataNotFound
	result, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, result.Bars)
	require.Equal(t, "关联期货暂无历史数据", result.ZScores[0].Warning)
}
