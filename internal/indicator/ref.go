package indicator

import (
	"errors"
	"fmt"

	"trading/internal/market"
)

var ErrInvalidRef = errors.New("indicator: invalid feature reference")

type Kind string

const (
	OHLC     Kind = "ohlc"
	SMAKind  Kind = "sma"
	EMAKind  Kind = "ema"
	VolumeMA Kind = "volume_ma"
	MACDKind Kind = "macd"
	KDJKind  Kind = "kdj"
)

type Field string

const (
	Open      Field = "open"
	High      Field = "high"
	Low       Field = "low"
	Close     Field = "close"
	Volume    Field = "volume"
	DIF       Field = "dif"
	DEA       Field = "dea"
	Histogram Field = "histogram"
	K         Field = "k"
	D         Field = "d"
	J         Field = "j"
)

// Ref identifies one fully-specified feature. Its key is stable across runs.
type Ref struct {
	Kind      Kind
	Timeframe market.Timeframe
	PriceView market.PriceView
	Field     Field
	Period    int
	Fast      int
	Slow      int
	Signal    int
}

func (r Ref) Validate() error {
	if !r.Timeframe.Valid() || !validPriceView(r.PriceView) {
		return ErrInvalidRef
	}
	switch r.Kind {
	case OHLC:
		if !isOHLC(r.Field) || r.hasParameters() {
			return ErrInvalidRef
		}
	case SMAKind, EMAKind:
		if !isOHLC(r.Field) || r.Period <= 0 || r.Fast != 0 || r.Slow != 0 || r.Signal != 0 {
			return ErrInvalidRef
		}
	case VolumeMA:
		if r.Field != Volume || r.PriceView != market.Raw || r.Period <= 0 || r.Fast != 0 || r.Slow != 0 || r.Signal != 0 {
			return ErrInvalidRef
		}
	case MACDKind:
		if !isMACDField(r.Field) || r.Fast <= 0 || r.Slow <= r.Fast || r.Signal <= 0 || r.Period != 0 {
			return ErrInvalidRef
		}
	case KDJKind:
		if !isKDJField(r.Field) || r.Period <= 0 || r.Fast != 0 || r.Slow != 0 || r.Signal != 0 {
			return ErrInvalidRef
		}
	default:
		return ErrInvalidRef
	}
	return nil
}

func (r Ref) hasParameters() bool {
	return r.Period != 0 || r.Fast != 0 || r.Slow != 0 || r.Signal != 0
}

func (r Ref) Key() string {
	if r.Validate() != nil {
		return ""
	}
	base := fmt.Sprintf("%s/%s/%s/%s", r.Kind, timeframeKey(r.Timeframe), priceViewKey(r.PriceView), r.Field)
	switch r.Kind {
	case SMAKind, EMAKind, VolumeMA, KDJKind:
		return fmt.Sprintf("%s/p=%d", base, r.Period)
	case MACDKind:
		return fmt.Sprintf("%s/f=%d/s=%d/sig=%d", base, r.Fast, r.Slow, r.Signal)
	default:
		return base
	}
}

func isOHLC(field Field) bool {
	return field == Open || field == High || field == Low || field == Close
}

func isMACDField(field Field) bool {
	return field == DIF || field == DEA || field == Histogram
}

func isKDJField(field Field) bool {
	return field == K || field == D || field == J
}

func validPriceView(view market.PriceView) bool {
	return view == market.Raw || view == market.ForwardAdjusted
}

func timeframeKey(tf market.Timeframe) string {
	switch tf {
	case market.Day:
		return "day"
	case market.Week:
		return "week"
	case market.Month:
		return "month"
	default:
		return "invalid"
	}
}

func priceViewKey(view market.PriceView) string {
	if view == market.ForwardAdjusted {
		return "qfq"
	}
	return "raw"
}
