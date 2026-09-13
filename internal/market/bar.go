package market

import "time"

type TradingStatus uint8

const (
	Tradable TradingStatus = iota
	Suspended
)

type Bar struct {
	Instrument InstrumentID
	Timeframe  Timeframe
	OpenTime   time.Time
	CloseTime  time.Time
	Open       Price
	High       Price
	Low        Price
	Close      Price
	Volume     int64
	Amount     Money
	Trading    TradingStatus
	LimitUp    *Price
	LimitDown  *Price
	Version    DataVersion
}
