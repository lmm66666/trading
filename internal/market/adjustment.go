package market

import (
	"sort"
	"time"
)

type AdjustmentFactor struct {
	EffectiveTime time.Time
	Numerator     int64
	Denominator   int64
	Version       DataVersion
}

type Alignment struct {
	Primary            Bar
	Auxiliary          Bar
	PrimaryCloseTime   time.Time
	AuxiliaryCloseTime time.Time
}

func AdjustedPrice(raw Price, at time.Time, factors []AdjustmentFactor) (float64, bool) {
	knownFactors := append([]AdjustmentFactor(nil), factors...)
	sort.SliceStable(knownFactors, func(i, j int) bool {
		return knownFactors[i].EffectiveTime.Before(knownFactors[j].EffectiveTime)
	})

	lastKnown := sort.Search(len(knownFactors), func(i int) bool {
		return knownFactors[i].EffectiveTime.After(at)
	}) - 1
	if lastKnown < 0 {
		return 0, false
	}

	factor := knownFactors[lastKnown]
	if factor.Numerator <= 0 || factor.Denominator <= 0 {
		return 0, false
	}
	adjusted := (float64(raw) / float64(ValueScale)) * float64(factor.Numerator) / float64(factor.Denominator)
	return adjusted, true
}

func AlignAsOf(primary, auxiliary Dataset) []Alignment {
	primaryBars := primary.Bars()
	auxiliaryBars := auxiliary.Bars()
	alignments := make([]Alignment, 0, len(primaryBars))

	for _, primaryBar := range primaryBars {
		index := sort.Search(len(auxiliaryBars), func(i int) bool {
			return auxiliaryBars[i].CloseTime.After(primaryBar.CloseTime)
		}) - 1
		if index < 0 {
			continue
		}

		auxiliaryBar := auxiliaryBars[index]
		alignments = append(alignments, Alignment{
			Primary:            primaryBar,
			Auxiliary:          auxiliaryBar,
			PrimaryCloseTime:   primaryBar.CloseTime,
			AuxiliaryCloseTime: auxiliaryBar.CloseTime,
		})
	}
	return alignments
}
