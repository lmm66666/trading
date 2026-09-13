package application

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
)

// 六个完整交易周，日线只含周一到周五，周线仅在周五收盘。
func finalStateFixture(t *testing.T) (*MarketIngestionService, *marketSourceFake, *marketReadFake, *marketWriterFake) {
	t.Helper()
	svc, src, data, writer := ingestionFixture(t)
	data.latest = 7
	data.stored = map[market.Timeframe][]market.Bar{}
	for week := 0; week < 6; week++ {
		for weekday := 0; weekday < 5; weekday++ {
			data.stored[market.Day] = append(data.stored[market.Day], marketBar(market.Day, 5+week*7+weekday))
		}
		data.stored[market.Week] = append(data.stored[market.Week], marketBar(market.Week, 9+week*7))
	}
	src.bars = map[market.Timeframe][]market.Bar{
		market.Day:  append([]market.Bar(nil), data.stored[market.Day]...),
		market.Week: append([]market.Bar(nil), data.stored[market.Week]...),
	}
	data.factors = append([]market.AdjustmentFactor(nil), src.factors[market.Day]...)
	svc.config.Clock = func() time.Time { return marketDate(45) }
	return svc, src, data, writer
}

func withoutMarketDates(bars []market.Bar, from, to time.Time) []market.Bar {
	var result []market.Bar
	for _, bar := range bars {
		if bar.CloseTime.Before(from) || bar.CloseTime.After(to) {
			result = append(result, bar)
		}
	}
	return result
}

func TestRefreshFinalStateRejectsKnownGapsStillMissingAfterRefetch(t *testing.T) {
	for _, kind := range []string{"daily_close", "entire_daily_week", "weekly_bar", "latest_weekly_bar"} {
		t.Run(kind, func(t *testing.T) {
			svc, src, data, writer := finalStateFixture(t)
			tf, from, to := market.Day, marketDate(9), marketDate(9)
			if kind == "entire_daily_week" {
				from = marketDate(5)
			}
			if kind == "weekly_bar" {
				tf = market.Week
			}
			if kind == "latest_weekly_bar" {
				tf = market.Week
				from = marketDate(44)
				to = from
			}
			data.stored[tf] = withoutMarketDates(data.stored[tf], from, to)
			src.bars[tf] = withoutMarketDates(src.bars[tf], from, to)
			requested := map[market.Timeframe]time.Time{}
			src.fetch = func(_ context.Context, tf market.Timeframe, from, to time.Time) error {
				requested[tf] = from
				return nil
			}
			_, err := svc.Refresh(context.Background(), marketID)
			require.ErrorIs(t, err, ErrIncompleteMarketData)
			require.Empty(t, writer.batches, "扩大请求后依然缺数据，不能发布 COMPLETE")
			if tf == market.Day {
				require.Equal(t, marketDate(9), requested[market.Day])
			} else if kind == "latest_weekly_bar" {
				require.Equal(t, marketDate(16), requested[market.Week])
			} else {
				require.Equal(t, marketDate(5), requested[market.Week])
			}
		})
	}
}

func TestRefreshFinalStateRejectsWeeklyRevisionConflictingWithRetainedDailyBar(t *testing.T) {
	svc, src, _, writer := finalStateFixture(t)
	// 最近20根日线始于1月19日，周线重抓起点退至1月16日。
	src.bars[market.Week][1].Close = 105000
	requested := map[market.Timeframe]time.Time{}
	src.fetch = func(_ context.Context, tf market.Timeframe, from, to time.Time) error {
		requested[tf] = from
		return nil
	}
	_, err := svc.Refresh(context.Background(), marketID)
	require.Equal(t, marketDate(19), requested[market.Day])
	require.Equal(t, marketDate(16), requested[market.Week])
	require.ErrorIs(t, err, ErrIncompleteMarketData)
	require.Empty(t, writer.batches, "新周线不能与本次未重抓的旧日线矛盾")
}

func TestRefreshFinalStateAcceptsRepairedGapAndConsistentRetainedBoundary(t *testing.T) {
	for _, kind := range []string{"missing_daily", "missing_weekly", "consistent_revision", "weekend_gaps", "new_unconfirmed_week"} {
		t.Run(kind, func(t *testing.T) {
			svc, src, data, writer := finalStateFixture(t)
			switch kind {
			case "missing_daily":
				data.stored[market.Day] = withoutMarketDates(data.stored[market.Day], marketDate(9), marketDate(9))
			case "missing_weekly":
				data.stored[market.Week] = withoutMarketDates(data.stored[market.Week], marketDate(9), marketDate(9))
			case "consistent_revision":
				data.stored[market.Day][9].Close = 105000
				src.bars[market.Week][1].Close = 105000
			case "new_unconfirmed_week":
				src.bars[market.Day] = append(src.bars[market.Day], marketBar(market.Day, 47))
				svc.config.Clock = func() time.Time { return marketDate(47) }
			}
			got, err := svc.Refresh(context.Background(), marketID)
			require.NoError(t, err)
			require.NotZero(t, got.Version)
			require.Len(t, writer.batches, 1)
			if kind == "consistent_revision" {
				require.EqualValues(t, 100000, data.stored[market.Week][1].Close)
			}
		})
	}
}

func TestRefreshFinalStateOnlyNewestISOWeekMayLackWeeklyBar(t *testing.T) {
	for _, scenario := range []struct{ name, oldFriday, newMonday, latestMonday string }{
		{"february", "2026-02-13", "2026-02-16", "2026-02-23"},
		{"iso_year_rollover", "2025-12-19", "2025-12-22", "2025-12-29"},
		{"week_spans_calendar_year", "2025-12-26", "2025-12-29", "2026-01-05"},
	} {
		for _, repair := range []bool{false, true} {
			name := scenario.name + "/missing_earlier_week"
			if repair {
				name = scenario.name + "/earlier_week_repaired"
			}
			t.Run(name, func(t *testing.T) {
				svc, src, data, writer := ingestionFixture(t)
				parse := func(value string) time.Time {
					at, err := time.Parse("2006-01-02", value)
					require.NoError(t, err)
					return at
				}
				oldFriday, newMonday, latestMonday := parse(scenario.oldFriday), parse(scenario.newMonday), parse(scenario.latestMonday)
				barAt := func(tf market.Timeframe, at time.Time) market.Bar {
					bar := marketBar(tf, 1)
					bar.OpenTime = at
					bar.CloseTime = at
					return bar
				}
				data.latest = 7
				data.stored = map[market.Timeframe][]market.Bar{
					market.Day: {barAt(market.Day, oldFriday)}, market.Week: {barAt(market.Week, oldFriday)},
				}
				src.bars = map[market.Timeframe][]market.Bar{
					market.Day:  append([]market.Bar(nil), data.stored[market.Day]...),
					market.Week: append([]market.Bar(nil), data.stored[market.Week]...),
				}
				for day := 0; day < 5; day++ {
					src.bars[market.Day] = append(src.bars[market.Day], barAt(market.Day, newMonday.AddDate(0, 0, day)))
				}
				src.bars[market.Day] = append(src.bars[market.Day], barAt(market.Day, latestMonday))
				if repair {
					src.bars[market.Week] = append(src.bars[market.Week], barAt(market.Week, newMonday.AddDate(0, 0, 4)))
				}
				factor := market.AdjustmentFactor{EffectiveTime: oldFriday, Numerator: 4, Denominator: 5}
				src.factors = map[market.Timeframe][]market.AdjustmentFactor{market.Day: {factor}, market.Week: {factor}}
				src.actions = nil
				data.factors = []market.AdjustmentFactor{factor}
				svc.config.HistoryStart = oldFriday
				svc.config.Clock = func() time.Time { return latestMonday }
				result, err := svc.Refresh(context.Background(), marketID)
				if repair {
					require.NoError(t, err)
					require.NotZero(t, result.Version)
					require.Len(t, writer.batches, 1)
				} else {
					require.ErrorIs(t, err, ErrIncompleteMarketData)
					require.Empty(t, writer.batches, "后续日线周组已经证明更早的新周结束，不得豁免缺失周线")
				}
			})
		}
	}
}
