package backtest

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

func TestFinalReasonMapsEveryExecutionOutcome(t *testing.T) {
	cases := map[RejectReason]OrderFinalReason{
		RejectNone:                     OrderFilled,
		RejectInvalidConfig:            UnfilledInvalidConfig,
		RejectInvalidOrder:             UnfilledInvalidOrder,
		RejectNotYetActive:             UnfilledNotYetActive,
		RejectNotTradable:              UnfilledNotTradable,
		RejectLimitUp:                  UnfilledLimitUp,
		RejectLimitDown:                UnfilledLimitDown,
		RejectInvalidLot:               UnfilledInvalidLot,
		RejectInsufficientCash:         UnfilledInsufficientCash,
		RejectInsufficientPosition:     UnfilledInsufficientPosition,
		RejectDuplicateFill:            UnfilledDuplicateFill,
		RejectArithmeticOverflow:       UnfilledArithmeticOverflow,
		RejectInvalidBar:               UnfilledInvalidBar,
		RejectFeesExceedProceeds:       UnfilledFeesExceedProceeds,
		RejectAccountOverflow:          UnfilledAccountOverflow,
		RejectInvalidAccountTransition: UnfilledInvalidAccountTransition,
	}
	for reason, want := range cases {
		assert.Equal(t, want, finalReason(reason))
	}
	assert.Equal(t, UnfilledInvalidAccountTransition, finalReason(RejectReason(255)))
}

func TestBarContextExposesOnlyConfirmedFeatureValues(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	bar := internalBar(id, "2026-01-05")
	dataset, err := market.NewDataset(id, market.Day, 1, []market.Bar{bar})
	require.NoError(t, err)
	ref := indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close, Period: 1}
	timeline, err := strategy.NewTimeline(dataset, indicator.Set{ref.Key(): indicator.SMA([]float64{2}, 1)}, nil)
	require.NoError(t, err)
	ctx := newBarContext(timeline, 0, strategy.PositionView{Open: true, Quantity: 100, HoldingBars: 1})
	assert.Equal(t, bar, ctx.Bar())
	assert.Equal(t, 0, ctx.Index())
	value, ok := ctx.Float(ref, 0)
	assert.True(t, ok)
	assert.Equal(t, 2.0, value)
	_, ok = ctx.Float(ref, 1)
	assert.False(t, ok)
	_, ok = ctx.Float(indicator.Ref{Timeframe: market.Week}, 0)
	assert.False(t, ok)
	_, ok = ctx.Float(indicator.Ref{Timeframe: market.Day}, 0)
	assert.False(t, ok)
	assert.Equal(t, strategy.PositionView{Open: true, Quantity: 100, HoldingBars: 1}, ctx.Position())
	_, ok = ctx.Float(ref, -1)
	assert.False(t, ok)
	assert.ErrorIs(t, ctx.Err(), strategy.ErrFutureAccess)
}

func TestBarContextReadsOnlyAsOfAuxiliaryFeature(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	primary := internalBar(id, "2026-01-05")
	primaryDataset, err := market.NewDataset(id, market.Day, 1, []market.Bar{primary})
	require.NoError(t, err)
	auxiliary := internalBar(id, "2026-01-05")
	auxiliary.Timeframe = market.Week
	auxiliaryDataset, err := market.NewDataset(id, market.Week, 1, []market.Bar{auxiliary})
	require.NoError(t, err)
	ref := indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close, Period: 1}
	timeline, err := strategy.NewTimeline(primaryDataset, nil, map[market.Timeframe]strategy.AlignedFeatures{
		market.Week: {Dataset: auxiliaryDataset, Features: indicator.Set{ref.Key(): indicator.SMA([]float64{3}, 1)}, PrimaryToAuxiliary: []int{0}},
	})
	require.NoError(t, err)
	value, ok := newBarContext(timeline, 0, strategy.PositionView{}).Float(ref, 0)
	assert.True(t, ok)
	assert.Equal(t, 3.0, value)
}

func TestEngineValidatesEmptyTimelineMissingFeaturesAndActionApplication(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	empty, err := market.NewDataset(id, market.Day, 1, nil)
	require.NoError(t, err)
	timeline, err := strategy.NewTimeline(empty, nil, nil)
	require.NoError(t, err)
	_, err = (Engine{}).Run(context.Background(), Input{Strategy: internalStrategy{}, Timeline: timeline, Config: internalConfig()})
	assert.ErrorIs(t, err, ErrInvalidInput)

	bar := internalBar(id, "2026-01-05")
	dataset, err := market.NewDataset(id, market.Day, 1, []market.Bar{bar})
	require.NoError(t, err)
	timeline, err = strategy.NewTimeline(dataset, nil, nil)
	require.NoError(t, err)
	_, err = (Engine{}).Run(context.Background(), Input{Strategy: featureStrategy{}, Timeline: timeline, Config: internalConfig()})
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, err = (Engine{}).Run(context.Background(), Input{
		Strategy: internalStrategy{}, Timeline: timeline, Config: internalConfig(),
		Actions: []market.CorporateAction{{ID: "rights", Instrument: id, Version: 1, ExDate: bar.OpenTime, Kind: market.RightsIssue}},
	})
	assert.ErrorIs(t, err, ErrUnsupportedCorporateAction)
}

func TestEngineRejectsNilContextAndInvalidDecision(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	timeline := internalTimeline(t, []market.Bar{internalBar(id, "2026-01-05")})
	_, err := (Engine{}).Run(nil, Input{Strategy: internalStrategy{}, Timeline: timeline, Config: internalConfig()})
	assert.ErrorIs(t, err, ErrInvalidInput)
	_, err = (Engine{}).Run(context.Background(), Input{Strategy: invalidDecisionStrategy{}, Timeline: timeline, Config: internalConfig()})
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestDefinitionValidationRejectsInvalidAndMissingAuxiliaryFeatures(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	timeline := internalTimeline(t, []market.Bar{internalBar(id, "2026-01-05")})
	for _, definition := range []strategy.Definition{
		{},
		{ID: "x", Version: "1", PrimaryTimeframe: market.Week},
		{ID: "x", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{{}}},
		{ID: "x", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{{Kind: indicator.SMAKind, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close, Period: 1}}},
	} {
		assert.ErrorIs(t, validateDefinition(definition, timeline), ErrInvalidInput)
	}
}

func TestTradeAndMetricBoundariesRemainFinite(t *testing.T) {
	entry := Fill{Side: Buy, Gross: 100, Commission: 1}
	exit := Fill{Side: Sell, Gross: 120, Commission: 1}
	trade, err := newTrade(entry, exit, 2)
	require.NoError(t, err)
	assert.Equal(t, market.Money(18), trade.NetProfit)
	_, err = newTrade(exit, entry, 0)
	assert.ErrorIs(t, err, ErrInvalidInput)
	_, err = newTrade(Fill{Side: Buy, Gross: math.MaxInt64, Commission: 1}, exit, 1)
	assert.ErrorIs(t, err, ErrAccountOverflow)
	_, err = newTrade(entry, Fill{Side: Sell, Gross: 0, Commission: 1}, 1)
	assert.ErrorIs(t, err, ErrAccountOverflow)
	_, ok := subtractMoney(-1, 1)
	assert.False(t, ok)

	points := []EquityPoint{
		{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Equity: 100},
		{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Equity: 121},
	}
	summary := calculateMetricsWithInitial(points, []Trade{{NetProfit: 1, HoldingBars: 1}}, strategy.PositionView{}, 100)
	require.NotNil(t, summary.AnnualizedReturn)
	assert.InDelta(t, 0.21, *summary.AnnualizedReturn, 0.01)
	firstBarLoss := calculateMetricsWithInitial([]EquityPoint{{Equity: 90}}, nil, strategy.PositionView{}, 100)
	assert.InDelta(t, 0.1, firstBarLoss.MaximumDrawdown, 1e-9)
	assert.False(t, finite(math.Inf(1)))
	assert.False(t, finite(math.NaN()))
}

func TestMarkToMarketChecksCashAndContextBeforeAppend(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	state := engineState{timeline: internalTimeline(t, []market.Bar{internalBar(id, "2026-01-05")}), account: Account{cash: math.MaxInt64, positions: map[market.InstrumentID]Position{id: {Instrument: id, Quantity: 1}}, appliedFillIDs: map[string]struct{}{}, appliedOrderIDs: map[string]struct{}{}, appliedActionIDs: map[string]struct{}{}}}
	err := state.markToMarketAtClose(context.Background(), 0)
	assert.ErrorIs(t, err, ErrAccountOverflow)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = state.markToMarketAtClose(ctx, 0)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMarkToMarketFailsOnUnrepresentablePositionValue(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	account, err := NewAccount(0)
	require.NoError(t, err)
	account.positions[id] = Position{Instrument: id, Quantity: math.MaxInt64}
	state := engineState{timeline: internalTimeline(t, []market.Bar{internalBar(id, "2026-01-05")}), account: account}
	_, err = state.positionValue(market.Price(2))
	assert.ErrorIs(t, err, ErrAccountOverflow)
}

type internalStrategy struct{}

func (internalStrategy) Definition() strategy.Definition {
	return strategy.Definition{ID: "internal", Version: "1", PrimaryTimeframe: market.Day, DefaultHoldBars: 10}
}
func (internalStrategy) OnBar(strategy.Context) (strategy.Decision, error) {
	return strategy.Decision{}, nil
}

type featureStrategy struct{ internalStrategy }

func (featureStrategy) Definition() strategy.Definition {
	return strategy.Definition{ID: "feature", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close, Period: 1}}}
}

type invalidDecisionStrategy struct{ internalStrategy }

func (invalidDecisionStrategy) OnBar(strategy.Context) (strategy.Decision, error) {
	return strategy.Decision{Action: strategy.Action(99)}, nil
}

func internalConfig() Config { return Config{InitialCash: 100, CashFractionBPS: 10_000, LotSize: 1} }

func internalTimeline(t *testing.T, bars []market.Bar) strategy.Timeline {
	t.Helper()
	dataset, err := market.NewDataset(bars[0].Instrument, market.Day, 1, bars)
	require.NoError(t, err)
	timeline, err := strategy.NewTimeline(dataset, nil, nil)
	require.NoError(t, err)
	return timeline
}

func internalBar(id market.InstrumentID, date string) market.Bar {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return market.Bar{Instrument: id, Timeframe: market.Day, Version: 1, OpenTime: day.Add(9 * time.Hour), CloseTime: day.Add(15 * time.Hour), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}
}
