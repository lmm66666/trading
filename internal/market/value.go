package market

const ValueScale int64 = 10_000

type Price int64

type Money int64

type DataVersion uint64

type PriceView uint8

const (
	Raw PriceView = iota
	ForwardAdjusted
)
