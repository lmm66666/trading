package strategy

import (
	"fmt"
)

// ReplayLatest advances one supplied strategy instance across one instrument's
// primary timeline and returns its final decision. Callers obtain a fresh
// instance from Registry for each instrument replay.
func ReplayLatest(strategy Strategy, timeline Timeline) (Decision, error) {
	if isNilStrategy(strategy) {
		return Decision{}, ErrInvalidDefinition
	}
	validated, err := NewTimeline(timeline.Primary, timeline.Features, timeline.Auxiliary)
	if err != nil {
		return Decision{}, err
	}
	definition := strategy.Definition()
	if !validDefinition(definition) || definition.PrimaryTimeframe != validated.Primary.Timeframe() {
		return Decision{}, ErrIncompatibleData
	}
	if err := ensureFeatures(definition, validated); err != nil {
		return Decision{}, err
	}
	latest := Decision{Action: Hold}
	for index := 0; index < validated.Len(); index++ {
		context := newContext(validated, index, PositionView{})
		decision, err := strategy.OnBar(context)
		if contextErr := context.Err(); contextErr != nil {
			return Decision{}, contextErr
		}
		if err != nil {
			return Decision{}, err
		}
		latest = decision
	}
	return latest, nil
}

func ensureFeatures(definition Definition, timeline Timeline) error {
	for _, ref := range definition.Features {
		if ref.Timeframe == timeline.Primary.Timeframe() {
			if _, exists := timeline.Features[ref.Key()]; !exists {
				return fmt.Errorf("%w: %s", ErrMissingFeature, ref.Key())
			}
			continue
		}
		aligned, exists := timeline.Auxiliary[ref.Timeframe]
		if !exists {
			return fmt.Errorf("%w: auxiliary %s", ErrMissingFeature, ref.Key())
		}
		if _, exists := aligned.Features[ref.Key()]; !exists {
			return fmt.Errorf("%w: %s", ErrMissingFeature, ref.Key())
		}
	}
	return nil
}
