package market_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/market"
)

func TestAdjustedCloseUsesLatestKnownFactor(t *testing.T) {
	factors := []market.AdjustmentFactor{
		{EffectiveTime: utc("2026-01-01"), Numerator: 8, Denominator: 10},
	}
	got, ok := market.AdjustedPrice(market.Price(100_000), utc("2026-01-02"), factors)
	assert.True(t, ok)
	assert.InDelta(t, 8.0, got, 0.0001)
}

func TestAdjustedPriceRejectsMissingOrInvalidFactor(t *testing.T) {
	_, ok := market.AdjustedPrice(100_000, utc("2025-12-31"), []market.AdjustmentFactor{{
		EffectiveTime: utc("2026-01-01"), Numerator: 1, Denominator: 1,
	}})
	assert.False(t, ok)

	_, ok = market.AdjustedPrice(100_000, utc("2026-01-01"), []market.AdjustmentFactor{{
		EffectiveTime: utc("2026-01-01"), Numerator: 0, Denominator: 1,
	}})
	assert.False(t, ok)
}

func TestAlignAsOfNeverReadsFutureAuxiliaryBar(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	primaryWeekly := mustDataset(t, id, market.Week, 7, []market.Bar{
		bar(id, market.Week, 7, "2026-01-02"),
		bar(id, market.Week, 7, "2026-01-10"),
	})
	auxiliaryDaily := mustDataset(t, id, market.Day, 7, []market.Bar{
		bar(id, market.Day, 7, "2026-01-02"),
		bar(id, market.Day, 7, "2026-01-09"),
		bar(id, market.Day, 7, "2026-01-12"),
	})

	got := market.AlignAsOf(primaryWeekly, auxiliaryDaily)
	require.Len(t, got, 2)
	assert.Equal(t, utc("2026-01-09"), got[1].AuxiliaryCloseTime)
	assert.True(t, got[1].AuxiliaryCloseTime.Before(got[1].PrimaryCloseTime) || got[1].AuxiliaryCloseTime.Equal(got[1].PrimaryCloseTime))
}

func TestAlignAsOfSkipsPrimaryBarsWithoutKnownAuxiliaryBar(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	primary := mustDataset(t, id, market.Week, 7, []market.Bar{bar(id, market.Week, 7, "2026-01-01")})
	auxiliary := mustDataset(t, id, market.Day, 7, []market.Bar{bar(id, market.Day, 7, "2026-01-02")})

	assert.Empty(t, market.AlignAsOf(primary, auxiliary))
}

func bar(id market.InstrumentID, tf market.Timeframe, version market.DataVersion, date string) market.Bar {
	value := testBar(id, date, 100000, 103000, 99000, 102000)
	value.Timeframe = tf
	value.Version = version
	return value
}

func mustDataset(t *testing.T, id market.InstrumentID, tf market.Timeframe, version market.DataVersion, bars []market.Bar) market.Dataset {
	t.Helper()
	dataset, err := market.NewDataset(id, tf, version, bars)
	require.NoError(t, err)
	return dataset
}
