package market

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAggregateWeeklyUsesObservedTradingWeek(t *testing.T) {
	id := InstrumentID{Exchange: SSE, Code: "600000"}
	daily := []Bar{
		dailyBarForWeek(id, "2026-09-07", 100, 130, 90, 120, 10, 1000, Tradable),
		dailyBarForWeek(id, "2026-09-08", 120, 150, 110, 140, 20, 2000, Suspended),
		dailyBarForWeek(id, "2026-09-11", 140, 145, 105, 110, 30, 3000, Tradable),
	}

	weekly, err := AggregateWeekly(id, daily)
	require.NoError(t, err)
	require.Len(t, weekly, 1)
	got := weekly[0]
	require.Equal(t, Week, got.Timeframe)
	require.Equal(t, Price(100), got.Open)
	require.Equal(t, Price(150), got.High)
	require.Equal(t, Price(90), got.Low)
	require.Equal(t, Price(110), got.Close)
	require.Equal(t, int64(60), got.Volume)
	require.Equal(t, Money(6000), got.Amount)
	require.Equal(t, Suspended, got.Trading)
	require.Equal(t, daily[0].OpenTime, got.OpenTime)
	require.Equal(t, daily[2].CloseTime, got.CloseTime)
}

func TestAggregateWeeklySupportsHolidayShortWeekAndUnsortedInput(t *testing.T) {
	id := InstrumentID{Exchange: SZSE, Code: "000001"}
	second := dailyBarForWeek(id, "2026-10-09", 120, 140, 110, 130, 2, 20, Tradable)
	first := dailyBarForWeek(id, "2026-10-08", 100, 130, 90, 120, 1, 10, Tradable)

	weekly, err := AggregateWeekly(id, []Bar{second, first})
	require.NoError(t, err)
	require.Len(t, weekly, 1)
	require.Equal(t, first.Open, weekly[0].Open)
	require.Equal(t, second.Close, weekly[0].Close)
	require.Equal(t, second.CloseTime, weekly[0].CloseTime)
}

func TestAggregateWeeklyRejectsDuplicateOrMismatchedDailyBars(t *testing.T) {
	id := InstrumentID{Exchange: SSE, Code: "600000"}
	bar := dailyBarForWeek(id, "2026-09-07", 100, 130, 90, 120, 10, 1000, Tradable)

	_, err := AggregateWeekly(id, []Bar{bar, bar})
	require.ErrorIs(t, err, ErrDuplicateBar)

	other := bar
	other.Instrument = InstrumentID{Exchange: SSE, Code: "600001"}
	_, err = AggregateWeekly(id, []Bar{other})
	require.ErrorIs(t, err, ErrBarInstrumentMismatch)

	wrongTimeframe := bar
	wrongTimeframe.Timeframe = Week
	_, err = AggregateWeekly(id, []Bar{wrongTimeframe})
	require.ErrorIs(t, err, ErrBarTimeframeMismatch)
}

func dailyBarForWeek(id InstrumentID, day string, open, high, low, close Price, volume int64, amount Money, status TradingStatus) Bar {
	at, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return Bar{Instrument: id, Timeframe: Day, OpenTime: at.UTC(), CloseTime: at.UTC(), Open: open, High: high, Low: low, Close: close, Volume: volume, Amount: amount, Trading: status}
}
