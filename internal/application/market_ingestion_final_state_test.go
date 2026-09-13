package application

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/market"
)

func TestRefreshFinalStateRejectsKnownDailyGapStillMissingFromOverlap(t *testing.T) {
	svc, src, data, writer := ingestionFixture(t)
	data.stored = map[market.Timeframe][]market.Bar{}
	for day := 1; day <= 30; day++ {
		data.stored[market.Day] = append(data.stored[market.Day], marketBar(market.Day, day))
	}
	data.latest = 7
	data.factors = append([]market.AdjustmentFactor(nil), src.factors[market.Day]...)
	src.bars[market.Day] = append([]market.Bar(nil), data.stored[market.Day]...)
	src.bars[market.Day] = withoutMarketDates(src.bars[market.Day], marketDate(20), marketDate(20))

	_, err := svc.Refresh(context.Background(), marketID)
	require.ErrorIs(t, err, ErrIncompleteMarketData)
	require.Empty(t, writer.batches)
}

func TestRefreshFinalStateDerivesAndRepairsWeeklyBarsFromDaily(t *testing.T) {
	svc, src, data, writer := ingestionFixture(t)
	data.latest = 7
	data.stored = map[market.Timeframe][]market.Bar{
		market.Day: {
			marketBar(market.Day, 5), marketBar(market.Day, 6), marketBar(market.Day, 7),
			marketBar(market.Day, 8), marketBar(market.Day, 9),
		},
		market.Week: {marketBar(market.Week, 8)},
	}
	data.factors = append([]market.AdjustmentFactor(nil), src.factors[market.Day]...)
	src.bars[market.Day] = append([]market.Bar(nil), data.stored[market.Day]...)

	_, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Len(t, writer.batches, 1)
	require.Len(t, writer.batches[0].Bars[market.Week], 1)
	require.Equal(t, marketDate(9), writer.batches[0].Bars[market.Week][0].CloseTime)
}

func TestRefreshFinalStateCoversHistoryBeforeFirstSinaFactor(t *testing.T) {
	svc, src, _, writer := ingestionFixture(t)
	src.bars[market.Day] = []market.Bar{marketBar(market.Day, 1), marketBar(market.Day, 2)}
	src.factors[market.Day] = []market.AdjustmentFactor{{EffectiveTime: marketDate(2), Numerator: 4, Denominator: 5}}

	_, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Equal(t, []market.AdjustmentFactor{
		{EffectiveTime: marketDate(1), Numerator: 4, Denominator: 5},
		{EffectiveTime: marketDate(2), Numerator: 4, Denominator: 5},
	}, writer.batches[0].Factors)
}

func TestRefreshFinalStatePublishesUnfinishedObservedWeek(t *testing.T) {
	svc, src, _, writer := ingestionFixture(t)
	monday := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)
	barAt := func(at time.Time) market.Bar {
		bar := marketBar(market.Day, 1)
		bar.OpenTime, bar.CloseTime = at, at
		return bar
	}
	src.bars[market.Day] = []market.Bar{barAt(monday), barAt(tuesday)}
	src.factors[market.Day] = []market.AdjustmentFactor{{EffectiveTime: monday, Numerator: 1, Denominator: 1}}
	svc.config.HistoryStart = monday
	svc.config.Clock = func() time.Time { return tuesday }

	_, err := svc.Refresh(context.Background(), marketID)
	require.NoError(t, err)
	require.Len(t, writer.batches[0].Bars[market.Week], 1)
	require.Equal(t, tuesday, writer.batches[0].Bars[market.Week][0].CloseTime)
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
