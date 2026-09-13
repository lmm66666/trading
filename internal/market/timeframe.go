package market

type Timeframe uint8

const (
	UnknownTimeframe Timeframe = iota
	Day
	Week
	Month
)

func (tf Timeframe) Valid() bool {
	return tf == Day || tf == Week || tf == Month
}
