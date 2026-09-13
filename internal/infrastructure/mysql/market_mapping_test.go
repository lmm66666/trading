package mysql

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func testBatch(close market.Price) port.MarketWriteBatch {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	return port.MarketWriteBatch{Source: "fixture", Instrument: id, Bars: map[market.Timeframe][]market.Bar{market.Day: {{Instrument: id, Timeframe: market.Day, OpenTime: time.Date(2026, 1, 2, 1, 30, 0, 0, time.UTC), CloseTime: time.Date(2026, 1, 2, 7, 0, 0, 0, time.UTC), Open: 100000, High: 120000, Low: 90000, Close: close, Volume: 100, Amount: 10000000}}}}
}

func TestCanonicalBatchOwnsSortsAndHashesContent(t *testing.T) {
	b := testBatch(100000)
	second := b.Bars[market.Day][0]
	second.CloseTime = second.CloseTime.AddDate(0, 0, 1)
	second.OpenTime = second.OpenTime.AddDate(0, 0, 1)
	b.Bars[market.Day] = append([]market.Bar{second}, b.Bars[market.Day]...)
	limit := market.Price(123000)
	b.Bars[market.Day][0].LimitUp = &limit
	canonical, digest, err := canonicalBatch(b)
	require.NoError(t, err)
	require.Len(t, digest, 64)
	require.True(t, canonical.Bars[market.Day][0].CloseTime.Before(canonical.Bars[market.Day][1].CloseTime))
	b.Bars[market.Day][0].Close = 1
	limit = 1
	require.Equal(t, market.Price(100000), canonical.Bars[market.Day][1].Close)
	require.Equal(t, market.Price(123000), *canonical.Bars[market.Day][1].LimitUp)
	canonical.Digest = digest
	_, same, err := canonicalBatch(canonical)
	require.NoError(t, err)
	require.Equal(t, digest, same)
	canonical.Bars[market.Day][0].Close = 110000
	_, _, err = canonicalBatch(canonical)
	require.Error(t, err)
}

func TestCanonicalBatchRejectsInvalidMarketObservations(t *testing.T) {
	for _, mutate := range []func(*port.MarketWriteBatch){
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].Close = 0 },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].CloseTime = time.Time{} },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].Trading = 99 },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].Amount = -1 },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day] = append(b.Bars[market.Day], b.Bars[market.Day][0]) },
		func(b *port.MarketWriteBatch) {
			b.Factors = []market.AdjustmentFactor{{EffectiveTime: b.Bars[market.Day][0].CloseTime, Numerator: 1, Denominator: 0}}
		},
		func(b *port.MarketWriteBatch) {
			b.Actions = []market.CorporateAction{{ID: "x", Instrument: b.Instrument, ExDate: b.Bars[market.Day][0].CloseTime, Kind: 99}}
		},
	} {
		b := testBatch(100000)
		mutate(&b)
		_, _, err := canonicalBatch(b)
		require.Error(t, err)
	}
}

func TestBarMappingPreservesRawMoneyAndPinnedVersion(t *testing.T) {
	b := testBatch(100000).Bars[market.Day][0]
	limit := market.Price(110000)
	b.LimitUp = &limit
	row := barModel(77, b, 3, 9)
	require.Equal(t, uint64(77), row.InstrumentID)
	require.Equal(t, uint32(3), row.Revision)
	got, err := row.bar(b.Instrument, 12)
	require.NoError(t, err)
	require.Equal(t, market.DataVersion(12), got.Version)
	require.Equal(t, b.Amount, got.Amount)
	*got.LimitUp = 1
	require.Equal(t, int64(110000), *row.LimitUp)
	row.Timeframe = "bad"
	_, err = row.bar(b.Instrument, 12)
	require.Error(t, err)
}

func TestInstrumentMappingStoresCanonicalIdentity(t *testing.T) {
	equity, err := instrumentModel(market.InstrumentID{Exchange: market.SSE, Code: "600000"}, "fixture")
	require.NoError(t, err)
	require.Equal(t, "SSE", equity.Exchange)
	require.Equal(t, "600000", equity.Code)
	require.Equal(t, "fixture", equity.Source)

	future, err := instrumentModel(market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}, "sina-futures")
	require.NoError(t, err)
	require.Equal(t, "SHFE", future.Exchange)
	require.Equal(t, "AU.MAIN", future.Code)
	require.Equal(t, "sina-futures", future.Source)

	_, err = instrumentModel(market.InstrumentID{Exchange: market.SHFE, Code: "AU202612"}, "fixture")
	require.Error(t, err)
}
