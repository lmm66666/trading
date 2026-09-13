package port_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/market"
	"trading/internal/port"
)

func TestFuturesPartitionCanonicalizesRowsAndRejectsDuplicates(t *testing.T) {
	day := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	au := portFuturesDaily(market.SHFE, "AU202612", "au2612", day)
	ag := portFuturesDaily(market.SHFE, "AG202612", "ag2612", day)
	input := port.FuturesPartition{Exchange: market.SHFE, TradeDate: day, Rows: []market.FuturesDaily{au, ag}, Provider: "akshare", ProviderVersion: "1.18.94"}
	canonical, err := port.CanonicalFuturesPartition(input)
	require.NoError(t, err)
	require.Equal(t, "AG202612", canonical.Rows[0].Instrument.Code)
	require.Equal(t, "AU202612", canonical.Rows[1].Instrument.Code)
	input.Rows[0].RawSymbol = "changed"
	require.Equal(t, "au2612", canonical.Rows[1].RawSymbol)

	input = canonical
	input.Rows = append(input.Rows, input.Rows[0])
	_, err = port.CanonicalFuturesPartition(input)
	require.Error(t, err)
}

func TestFuturesPartitionRejectsMismatchedAndAggregateRows(t *testing.T) {
	day := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	for _, rawSymbol := range []string{"0", "88", "888", "99"} {
		row := portFuturesDaily(market.SHFE, "AU202612", rawSymbol, day)
		_, err := port.CanonicalFuturesPartition(port.FuturesPartition{Exchange: market.SHFE, TradeDate: day, Rows: []market.FuturesDaily{row}, Provider: "akshare", ProviderVersion: "1.18.94"})
		require.Error(t, err)
	}
	row := portFuturesDaily(market.INE, "SC202612", "sc2612", day)
	_, err := port.CanonicalFuturesPartition(port.FuturesPartition{Exchange: market.SHFE, TradeDate: day, Rows: []market.FuturesDaily{row}, Provider: "akshare", ProviderVersion: "1.18.94"})
	require.Error(t, err)
}

func portFuturesDaily(exchange market.Exchange, code, raw string, day time.Time) market.FuturesDaily {
	id := market.InstrumentID{Exchange: exchange, Code: code}
	return market.FuturesDaily{
		Bar:        market.Bar{Instrument: id, Timeframe: market.Day, OpenTime: day.Add(time.Hour), CloseTime: day.Add(7 * time.Hour), Open: 10000, High: 11000, Low: 9000, Close: 10500, Volume: 10, Amount: 100000, Trading: market.Tradable},
		Instrument: id, TradeDate: day, PreSettlement: 9900, Settlement: 10400, VolumeLots: 10, OpenInterestLots: 20, Turnover: 100000,
		RawSymbol: raw, Product: id.Product(), StatisticsBasis: "EXCHANGE_DAILY", ProviderVersion: "1.18.94",
	}
}
