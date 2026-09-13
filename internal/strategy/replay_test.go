package strategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
	"trading/internal/market"
)

func TestReplayLatestRunsEveryBarInOrderAndReturnsLatestDecision(t *testing.T) {
	timeline := testTimeline(t)
	seen := make([]int, 0, timeline.Len())
	strategy := &testStrategy{
		definition: Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{closeRef}},
		onBar: func(ctx Context) (Decision, error) {
			seen = append(seen, ctx.Index())
			return Decision{Action: EnterLong, Reason: "latest"}, nil
		},
	}

	decision, err := ReplayLatest(strategy, timeline)

	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, seen)
	assert.Equal(t, Decision{Action: EnterLong, Reason: "latest"}, decision)
}

func TestReplayLatestFailsWhenStrategyIgnoresFutureAccessFailure(t *testing.T) {
	strategy := &testStrategy{
		definition: Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{closeRef}},
		onBar: func(ctx Context) (Decision, error) {
			_, _ = ctx.Float(closeRef, -1)
			return Decision{Action: Hold}, nil
		},
	}

	_, err := ReplayLatest(strategy, testTimeline(t))

	assert.ErrorIs(t, err, ErrFutureAccess)
}

func TestReplayLatestRejectsDefinitionFeatureMissingFromTimeline(t *testing.T) {
	strategy := &testStrategy{definition: Definition{
		ID: "test", Version: "1", PrimaryTimeframe: market.Day,
		Features: []indicator.Ref{{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close, Period: 2}},
	}}

	_, err := ReplayLatest(strategy, testTimeline(t))

	assert.ErrorIs(t, err, ErrMissingFeature)
}

func TestReplayLatestReturnsStrategyErrorAndRejectsIncompatibleDefinition(t *testing.T) {
	strategyError := &testStrategy{
		definition: Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{closeRef}},
		onBar:      func(Context) (Decision, error) { return Decision{}, assert.AnError },
	}
	_, err := ReplayLatest(strategyError, testTimeline(t))
	assert.ErrorIs(t, err, assert.AnError)

	wrongTimeframe := &testStrategy{definition: Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Week}}
	_, err = ReplayLatest(wrongTimeframe, testTimeline(t))
	assert.ErrorIs(t, err, ErrIncompatibleData)
}

func TestReplayLatestRequiresAuxiliaryFeatureDeclaredByStrategy(t *testing.T) {
	auxRef := indicator.Ref{Kind: indicator.OHLC, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close}
	strategy := &testStrategy{definition: Definition{
		ID: "test", Version: "1", PrimaryTimeframe: market.Day,
		Auxiliary: []market.Timeframe{market.Week}, Features: []indicator.Ref{auxRef},
	}}

	_, err := ReplayLatest(strategy, testTimeline(t))

	assert.ErrorIs(t, err, ErrMissingFeature)
}

func testTimeline(t *testing.T) Timeline {
	t.Helper()
	primary := testDataset(t, market.Day, []string{"2026-01-01", "2026-01-02", "2026-01-03"})
	timeline, err := NewTimeline(primary, indicator.Set{closeRef.Key(): indicator.SMA([]float64{10, 11, 12}, 1)}, nil)
	require.NoError(t, err)
	return timeline
}
