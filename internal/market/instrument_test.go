package market_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/market"
)

func TestInstrumentIdentityClassifiesEquityFuturesAndContinuous(t *testing.T) {
	tests := []struct {
		value      string
		want       market.InstrumentID
		assetClass market.AssetClass
		kind       market.InstrumentKind
		product    string
		delivery   time.Time
	}{
		{"SSE:600000", market.InstrumentID{Exchange: market.SSE, Code: "600000"}, market.Equity, market.SpotEquity, "", time.Time{}},
		{"SHFE:AU202612", market.InstrumentID{Exchange: market.SHFE, Code: "AU202612"}, market.Futures, market.FuturesContract, "AU", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{"INE:SC.MAIN", market.InstrumentID{Exchange: market.INE, Code: "SC.MAIN"}, market.Futures, market.FuturesContinuous, "SC", time.Time{}},
		{"DCE:JM202701", market.InstrumentID{Exchange: market.DCE, Code: "JM202701"}, market.Futures, market.FuturesContract, "JM", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"CZCE:ZC.MAIN", market.InstrumentID{Exchange: market.CZCE, Code: "ZC.MAIN"}, market.Futures, market.FuturesContinuous, "ZC", time.Time{}},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := market.ParseInstrumentID(test.value)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
			require.Equal(t, test.value, got.String())
			require.Equal(t, test.assetClass, got.AssetClass())
			require.Equal(t, test.kind, got.Kind())
			require.Equal(t, test.product, got.Product())
			delivery, ok := got.DeliveryMonth()
			require.Equal(t, !test.delivery.IsZero(), ok)
			require.Equal(t, test.delivery, delivery)
			roundTrip, err := market.ParseInstrumentID(got.String())
			require.NoError(t, err)
			require.Equal(t, got, roundTrip)
		})
	}
}

func TestInstrumentIdentityRejectsInvalidFuturesCodes(t *testing.T) {
	for _, value := range []string{
		"SHFE:au202612",
		"SHFE:AU2612",
		"SHFE:AU202600",
		"SHFE:AU202613",
		"SHFE:XX202612",
		"INE:AU202612",
		"DCE:ZC202612",
		"CZCE:JM202612",
		"SSE:AU202612",
		"SHFE:600000",
		"SHFE:AU.main",
		"SHFE:AU888",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := market.ParseInstrumentID(value)
			require.Error(t, err)
			require.True(t, errors.Is(err, market.ErrInvalidInstrumentCode) || errors.Is(err, market.ErrInvalidExchange))
		})
	}
}
