package market

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrInvalidTimeframe      = errors.New("market: invalid timeframe")
	ErrDuplicateBar          = errors.New("market: duplicate bar")
	ErrBarInstrumentMismatch = errors.New("market: bar instrument mismatch")
	ErrBarTimeframeMismatch  = errors.New("market: bar timeframe mismatch")
	ErrBarVersionMismatch    = errors.New("market: bar version mismatch")
	ErrInvalidOHLC           = errors.New("market: invalid OHLC")
	ErrNegativeVolume        = errors.New("market: negative volume")
)

type Dataset struct {
	instrument InstrumentID
	timeframe  Timeframe
	version    DataVersion
	bars       []Bar
}

func NewDataset(id InstrumentID, tf Timeframe, version DataVersion, bars []Bar) (Dataset, error) {
	if err := id.Validate(); err != nil {
		return Dataset{}, err
	}
	if !tf.Valid() {
		return Dataset{}, ErrInvalidTimeframe
	}

	sortedBars := cloneBars(bars)
	sort.SliceStable(sortedBars, func(i, j int) bool {
		return sortedBars[i].CloseTime.Before(sortedBars[j].CloseTime)
	})

	for i, bar := range sortedBars {
		if i > 0 && bar.CloseTime.Equal(sortedBars[i-1].CloseTime) {
			return Dataset{}, fmt.Errorf("%w: %s", ErrDuplicateBar, bar.CloseTime)
		}
		if err := validateBar(id, tf, version, bar); err != nil {
			return Dataset{}, err
		}
	}

	return Dataset{instrument: id, timeframe: tf, version: version, bars: sortedBars}, nil
}

func (d Dataset) Instrument() InstrumentID {
	return d.instrument
}

func (d Dataset) Timeframe() Timeframe {
	return d.timeframe
}

func (d Dataset) Version() DataVersion {
	return d.version
}

func (d Dataset) Len() int {
	return len(d.bars)
}

func (d Dataset) Bar(i int) Bar {
	return cloneBar(d.bars[i])
}

func (d Dataset) Bars() []Bar {
	return cloneBars(d.bars)
}

func validateBar(id InstrumentID, tf Timeframe, version DataVersion, bar Bar) error {
	if bar.Instrument != id {
		return ErrBarInstrumentMismatch
	}
	if bar.Timeframe != tf {
		return ErrBarTimeframeMismatch
	}
	if bar.Version != version {
		return ErrBarVersionMismatch
	}
	if bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 ||
		bar.Low > bar.Open || bar.Low > bar.Close || bar.Open > bar.High || bar.Close > bar.High {
		return ErrInvalidOHLC
	}
	if bar.Volume < 0 {
		return ErrNegativeVolume
	}
	return nil
}

func cloneBars(bars []Bar) []Bar {
	cloned := make([]Bar, len(bars))
	for i, bar := range bars {
		cloned[i] = cloneBar(bar)
	}
	return cloned
}

func cloneBar(bar Bar) Bar {
	bar.LimitUp = clonePrice(bar.LimitUp)
	bar.LimitDown = clonePrice(bar.LimitDown)
	return bar
}

func clonePrice(value *Price) *Price {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
