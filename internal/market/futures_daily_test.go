package market_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/market"
)

func TestFuturesDailyValidateAcceptsCompleteExchangeObservation(t *testing.T) {
	row := validFuturesDaily()
	require.NoError(t, row.Validate())
}

func TestFuturesDailyValidateRejectsInvalidFields(t *testing.T) {
	mutations := []func(*market.FuturesDaily){
		func(row *market.FuturesDaily) {
			row.Instrument = market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}
		},
		func(row *market.FuturesDaily) { row.TradeDate = row.TradeDate.Add(time.Hour) },
		func(row *market.FuturesDaily) {
			row.Bar.Instrument = market.InstrumentID{Exchange: market.SHFE, Code: "AG202612"}
		},
		func(row *market.FuturesDaily) { row.Bar.Timeframe = market.Week },
		func(row *market.FuturesDaily) { row.PreSettlement = 0 },
		func(row *market.FuturesDaily) { row.Settlement = -1 },
		func(row *market.FuturesDaily) { row.VolumeLots = -1 },
		func(row *market.FuturesDaily) { row.OpenInterestLots = -1 },
		func(row *market.FuturesDaily) { row.Turnover = -1 },
		func(row *market.FuturesDaily) { row.Bar.Volume++ },
		func(row *market.FuturesDaily) { row.Bar.Amount++ },
		func(row *market.FuturesDaily) { row.RawSymbol = "AU 2612" },
		func(row *market.FuturesDaily) { row.Product = "au" },
		func(row *market.FuturesDaily) { row.StatisticsBasis = "" },
		func(row *market.FuturesDaily) { row.ProviderVersion = "" },
	}
	for index, mutate := range mutations {
		row := validFuturesDaily()
		mutate(&row)
		require.Error(t, row.Validate(), "mutation %d", index)
	}
}

func validFuturesDaily() market.FuturesDaily {
	id := market.InstrumentID{Exchange: market.SHFE, Code: "AU202612"}
	day := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	return market.FuturesDaily{
		Bar: market.Bar{
			Instrument: id,
			Timeframe:  market.Day,
			OpenTime:   day.Add(time.Hour),
			CloseTime:  day.Add(7 * time.Hour),
			Open:       6123400,
			High:       6182000,
			Low:        6101200,
			Close:      6168800,
			Volume:     123456,
			Amount:     98765432000000,
			Trading:    market.Tradable,
		},
		Instrument:       id,
		TradeDate:        day,
		PreSettlement:    6112000,
		Settlement:       6154200,
		VolumeLots:       123456,
		OpenInterestLots: 198765,
		Turnover:         98765432000000,
		RawSymbol:        "au2612",
		Product:          "AU",
		StatisticsBasis:  "EXCHANGE_DAILY",
		ProviderVersion:  "1.18.94",
	}
}
