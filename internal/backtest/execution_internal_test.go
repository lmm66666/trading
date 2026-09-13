package backtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"trading/internal/market"
)

func TestExecutionReportsInvalidLotSeparatelyFromOtherConfigurationErrors(t *testing.T) {
	bar := market.Bar{
		Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600000"}, OpenTime: time.Date(2026, 1, 6, 9, 30, 0, 0, time.UTC), CloseTime: time.Date(2026, 1, 6, 15, 0, 0, 0, time.UTC),
		Open: 1, Low: 1, High: 1, Close: 1, Volume: 1, Trading: market.Tradable,
	}
	account, _ := NewAccount(1)
	order := NewNextOpenOrder(Buy, time.Date(2026, 1, 5, 15, 0, 0, 0, time.UTC), "signal")

	_, lotReason := (ExecutionModel{config: Config{InitialCash: 1, CashFractionBPS: 10_000}}).Execute(order, bar, account)
	_, configReason := (ExecutionModel{config: Config{InitialCash: 1, CashFractionBPS: 0, LotSize: 1}}).Execute(order, bar, account)

	assert.Equal(t, RejectInvalidLot, lotReason)
	assert.Equal(t, RejectInvalidConfig, configReason)
}
