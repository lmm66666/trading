package port_test

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func TestCanonicalMarketBatchOwnsAndValidatesPublication(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	at := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	b := port.MarketWriteBatch{Source: "test", Instrument: id, Factors: []market.AdjustmentFactor{{EffectiveTime: at, Numerator: 1, Denominator: 1, Version: 9}}}
	got, digest, err := port.CanonicalMarketBatch(b)
	require.NoError(t, err)
	require.Len(t, digest, 64)
	require.Zero(t, got.Factors[0].Version)
	require.EqualValues(t, 9, b.Factors[0].Version)
	got.Factors[0].Numerator = 2
	require.EqualValues(t, 1, b.Factors[0].Numerator)
	b.Digest = "wrong"
	_, _, err = port.CanonicalMarketBatch(b)
	require.ErrorIs(t, err, port.ErrInvalidPortValue)
}
func TestCanonicalMarketBatchRejectsInvalidObservations(t *testing.T) {
	at := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	fixture := func() port.MarketWriteBatch {
		return port.MarketWriteBatch{Source: "fixture", Instrument: id, Bars: map[market.Timeframe][]market.Bar{market.Day: {{Instrument: id, Timeframe: market.Day, OpenTime: at, CloseTime: at, Open: 10, High: 11, Low: 9, Close: 10, Volume: 1}}}, Factors: []market.AdjustmentFactor{{EffectiveTime: at, Numerator: 1, Denominator: 1}}, Actions: []market.CorporateAction{{ID: "dividend", Instrument: id, ExDate: at, Kind: market.CashDividend}}}
	}
	for _, mutate := range []func(*port.MarketWriteBatch){
		func(b *port.MarketWriteBatch) { b.Source = "" },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].Close = 0 },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].OpenTime = time.Time{} },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].CloseTime = at.Add(-time.Hour) },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].Trading = 99 },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day][0].Amount = -1 },
		func(b *port.MarketWriteBatch) { p := market.Price(0); b.Bars[market.Day][0].LimitUp = &p },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day] = append(b.Bars[market.Day], b.Bars[market.Day][0]) },
		func(b *port.MarketWriteBatch) { b.Factors = append(b.Factors, b.Factors[0]) },
		func(b *port.MarketWriteBatch) { b.Factors[0].EffectiveTime = at.Add(time.Nanosecond) },
		func(b *port.MarketWriteBatch) { b.Actions = append(b.Actions, b.Actions[0]) },
		func(b *port.MarketWriteBatch) { b.Actions[0].Kind = 99 },
		func(b *port.MarketWriteBatch) { b.Actions[0].ExDate = at.Add(time.Nanosecond) },
		func(b *port.MarketWriteBatch) { b.Actions[0].CashPerShare = -1 },
		func(b *port.MarketWriteBatch) { b.Actions[0].ShareNumerator = 1 },
		func(b *port.MarketWriteBatch) { b.Actions[0].Kind = market.ShareDistribution },
		func(b *port.MarketWriteBatch) { b.Bars[market.Day] = nil; b.Factors = nil; b.Actions = nil },
	} {
		b := fixture()
		mutate(&b)
		_, _, err := port.CanonicalMarketBatch(b)
		require.Error(t, err)
	}
	b := fixture()
	b.Actions[0].Kind = market.ShareDistribution
	b.Actions[0].ShareNumerator = 1
	b.Actions[0].ShareDenominator = 10
	digest, err := port.MarketBatchDigest(b)
	require.NoError(t, err)
	require.Len(t, digest, 64)
	b.Actions[0].Kind = market.RightsIssue
	_, err = port.MarketBatchDigest(b)
	require.NoError(t, err)
	require.Error(t, port.ValidateCorporateAction(market.CorporateAction{}))
	b.Actions[0].Instrument = market.InstrumentID{}
	require.Error(t, port.ValidateCorporateAction(b.Actions[0]))
}
