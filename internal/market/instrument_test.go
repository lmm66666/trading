package market_test

import (
	"errors"
	"testing"

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
	}{
		{"SSE:600000", market.InstrumentID{Exchange: market.SSE, Code: "600000"}, market.Equity, market.SpotEquity, ""},
		{"INE:SC.MAIN", market.InstrumentID{Exchange: market.INE, Code: "SC.MAIN"}, market.Futures, market.FuturesContinuous, "SC"},
		{"CZCE:ZC.MAIN", market.InstrumentID{Exchange: market.CZCE, Code: "ZC.MAIN"}, market.Futures, market.FuturesContinuous, "ZC"},
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
			roundTrip, err := market.ParseInstrumentID(got.String())
			require.NoError(t, err)
			require.Equal(t, got, roundTrip)
		})
	}
}

func TestInstrumentIdentityRejectsInvalidFuturesCodes(t *testing.T) {
	for _, value := range []string{
		"SHFE:AU202612",
		"SHFE:au.MAIN",
		"SHFE:XX.MAIN",
		"INE:AU.MAIN",
		"DCE:ZC.MAIN",
		"CZCE:JM.MAIN",
		"SSE:AU.MAIN",
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
