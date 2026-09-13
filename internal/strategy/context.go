package strategy

import (
	"fmt"
	"strconv"
	"strings"

	"trading/internal/indicator"
	"trading/internal/market"
)

// Timeline joins primary bars and their precomputed features with explicitly
// as-of aligned auxiliary datasets.
type Timeline struct {
	Primary     market.Dataset
	Features    indicator.Set
	Auxiliary   map[market.Timeframe]AlignedFeatures
	featureRefs map[indicator.Ref]indicator.Series
}

// AlignedFeatures maps each primary bar index to the latest confirmed
// auxiliary bar index, or -1 when no auxiliary bar is confirmed yet.
type AlignedFeatures struct {
	Dataset            market.Dataset
	Features           indicator.Set
	PrimaryToAuxiliary []int
	featureRefs        map[indicator.Ref]indicator.Series
}

// NewTimeline defensively copies mappings and validates every feature length
// and cross-timeframe as-of relation before a strategy can consume it.
func NewTimeline(primary market.Dataset, features indicator.Set, auxiliary map[market.Timeframe]AlignedFeatures) (Timeline, error) {
	if err := validateDataset(primary); err != nil {
		return Timeline{}, err
	}
	primaryRefs, err := validateFeatureSet(features, primary.Timeframe(), primary.Len())
	if err != nil {
		return Timeline{}, err
	}
	clonedAuxiliary := make(map[market.Timeframe]AlignedFeatures, len(auxiliary))
	for timeframe, aligned := range auxiliary {
		if !timeframe.Valid() || validateDataset(aligned.Dataset) != nil || aligned.Dataset.Timeframe() != timeframe || aligned.Dataset.Instrument() != primary.Instrument() {
			return Timeline{}, fmt.Errorf("%w: invalid auxiliary dataset", ErrInvalidTimeline)
		}
		featureRefs, err := validateFeatureSet(aligned.Features, aligned.Dataset.Timeframe(), aligned.Dataset.Len())
		if err != nil {
			return Timeline{}, err
		}
		if len(aligned.PrimaryToAuxiliary) != primary.Len() {
			return Timeline{}, fmt.Errorf("%w: auxiliary mapping length", ErrInvalidTimeline)
		}
		previous := -1
		for primaryIndex, auxiliaryIndex := range aligned.PrimaryToAuxiliary {
			if auxiliaryIndex < -1 || auxiliaryIndex >= aligned.Dataset.Len() || auxiliaryIndex < previous {
				return Timeline{}, fmt.Errorf("%w: auxiliary mapping index", ErrInvalidTimeline)
			}
			primaryCloseTime := primary.Bar(primaryIndex).CloseTime
			if auxiliaryIndex == -1 {
				if aligned.Dataset.Len() > 0 && !aligned.Dataset.Bar(0).CloseTime.After(primaryCloseTime) {
					return Timeline{}, fmt.Errorf("%w: auxiliary mapping is stale", ErrInvalidTimeline)
				}
			} else {
				if aligned.Dataset.Bar(auxiliaryIndex).CloseTime.After(primaryCloseTime) {
					return Timeline{}, fmt.Errorf("%w: auxiliary feature is from the future", ErrInvalidTimeline)
				}
				nextIndex := auxiliaryIndex + 1
				if nextIndex < aligned.Dataset.Len() && !aligned.Dataset.Bar(nextIndex).CloseTime.After(primaryCloseTime) {
					return Timeline{}, fmt.Errorf("%w: auxiliary mapping is stale", ErrInvalidTimeline)
				}
			}
			previous = auxiliaryIndex
		}
		clonedAuxiliary[timeframe] = AlignedFeatures{
			Dataset:            aligned.Dataset,
			Features:           cloneFeatureSet(aligned.Features),
			PrimaryToAuxiliary: append([]int(nil), aligned.PrimaryToAuxiliary...),
			featureRefs:        featureRefs,
		}
	}
	return Timeline{Primary: primary, Features: cloneFeatureSet(features), Auxiliary: clonedAuxiliary, featureRefs: primaryRefs}, nil
}

func (t Timeline) Len() int {
	return t.Primary.Len()
}

func validateDataset(dataset market.Dataset) error {
	if dataset.Instrument().Validate() != nil || !dataset.Timeframe().Valid() {
		return fmt.Errorf("%w: invalid dataset", ErrInvalidTimeline)
	}
	return nil
}

func validateFeatureSet(features indicator.Set, timeframe market.Timeframe, wantLength int) (map[indicator.Ref]indicator.Series, error) {
	refs := make(map[indicator.Ref]indicator.Series, len(features))
	for key, series := range features {
		ref, valid := parseFeatureKey(key)
		if !valid || ref.Timeframe != timeframe || series.Len() != wantLength {
			return nil, fmt.Errorf("%w: feature key or length", ErrInvalidTimeline)
		}
		refs[ref] = series
	}
	return refs, nil
}

func validFeatureKey(key string) bool {
	_, valid := parseFeatureKey(key)
	return valid
}

func parseFeatureKey(key string) (indicator.Ref, bool) {
	parts := strings.Split(key, "/")
	if len(parts) < 4 {
		return indicator.Ref{}, false
	}
	ref := indicator.Ref{Kind: indicator.Kind(parts[0]), Timeframe: parseTimeframe(parts[1]), PriceView: parsePriceView(parts[2]), Field: indicator.Field(parts[3])}
	switch ref.Kind {
	case indicator.OHLC:
		if len(parts) != 4 {
			return indicator.Ref{}, false
		}
	case indicator.SMAKind, indicator.EMAKind, indicator.VolumeMA, indicator.KDJKind:
		if len(parts) != 5 {
			return indicator.Ref{}, false
		}
		period, ok := featureParameter(parts[4], "p=")
		if !ok {
			return indicator.Ref{}, false
		}
		ref.Period = period
	case indicator.MACDKind:
		if len(parts) != 7 {
			return indicator.Ref{}, false
		}
		fast, fastOK := featureParameter(parts[4], "f=")
		slow, slowOK := featureParameter(parts[5], "s=")
		signal, signalOK := featureParameter(parts[6], "sig=")
		if !fastOK || !slowOK || !signalOK {
			return indicator.Ref{}, false
		}
		ref.Fast, ref.Slow, ref.Signal = fast, slow, signal
	default:
		return indicator.Ref{}, false
	}
	if ref.Validate() != nil || ref.Key() != key {
		return indicator.Ref{}, false
	}
	return ref, true
}

func parseTimeframe(value string) market.Timeframe {
	switch value {
	case "day":
		return market.Day
	case "week":
		return market.Week
	case "month":
		return market.Month
	default:
		return market.UnknownTimeframe
	}
}

func parsePriceView(value string) market.PriceView {
	if value == "qfq" {
		return market.ForwardAdjusted
	}
	return market.Raw
}

func featureParameter(value, prefix string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimPrefix(value, prefix))
	return parsed, err == nil && strings.HasPrefix(value, prefix)
}

func cloneFeatureSet(features indicator.Set) indicator.Set {
	cloned := make(indicator.Set, len(features))
	for key, series := range features {
		cloned[key] = series
	}
	return cloned
}

type contextView struct {
	timeline Timeline
	index    int
	position PositionView
	err      error
}

func newContext(timeline Timeline, index int, position PositionView) *contextView {
	return &contextView{timeline: timeline, index: index, position: position}
}

func (c *contextView) Bar() market.Bar {
	return c.timeline.Primary.Bar(c.index)
}

func (c *contextView) Index() int {
	return c.index
}

func (c *contextView) Float(ref indicator.Ref, ago int) (float64, bool) {
	if ago < 0 {
		c.err = ErrFutureAccess
		return 0, false
	}
	primaryIndex := c.index - ago
	if primaryIndex < 0 {
		return 0, false
	}
	if ref.Timeframe == c.timeline.Primary.Timeframe() {
		series, exists := c.timeline.featureRefs[ref]
		if !exists {
			return 0, false
		}
		return series.At(primaryIndex)
	}
	aligned, exists := c.timeline.Auxiliary[ref.Timeframe]
	if !exists || primaryIndex >= len(aligned.PrimaryToAuxiliary) {
		return 0, false
	}
	auxiliaryIndex := aligned.PrimaryToAuxiliary[primaryIndex]
	if auxiliaryIndex < 0 {
		return 0, false
	}
	series, exists := aligned.featureRefs[ref]
	if !exists {
		return 0, false
	}
	return series.At(auxiliaryIndex)
}

func (c *contextView) Position() PositionView {
	return c.position
}

func (c *contextView) Err() error {
	return c.err
}
