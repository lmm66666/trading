package backtest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/internal/backtest"
	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

func TestEngineFillsSignalAtNextOpen(t *testing.T) {
	result := runScripted(t, []strategy.Action{strategy.EnterLong, strategy.Hold}, nil)
	require.Len(t, result.Fills, 1)
	assert.Equal(t, engineOpenTime("2026-01-06"), result.Fills[0].Time)
	assert.Equal(t, backtest.OrderFilled, result.Orders[0].FinalReason)
}

func TestEngineAppliesCorporateActionBeforeOpenAndMarksRawClose(t *testing.T) {
	id := engineInstrument()
	action := market.CorporateAction{
		ID:           "dividend",
		Instrument:   id,
		ExDate:       engineOpenTime("2026-01-07"),
		Kind:         market.CashDividend,
		CashPerShare: 1_000,
		Version:      1,
	}
	result := runScriptedBars(t, []market.Bar{
		engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000),
		engineBar("2026-01-06", 100_000, 100_000, 100_000, 100_000),
		engineBar("2026-01-07", 100_000, 100_000, 100_000, 100_000),
	}, []strategy.Action{strategy.EnterLong, strategy.Hold, strategy.Hold}, []market.CorporateAction{action}, 0)
	require.Len(t, result.Equity, 3)
	assert.Equal(t, market.Money(1_000_000), result.Equity[2].Cash)
	assert.Equal(t, market.Money(100_000_000), result.Equity[2].PositionValue)
}

func TestLastBarDecisionRemainsUnfilled(t *testing.T) {
	result := runScripted(t, []strategy.Action{strategy.Hold, strategy.EnterLong}, nil)
	assert.Empty(t, result.Fills)
	require.Len(t, result.Orders, 1)
	assert.Equal(t, backtest.UnfilledNoNextBar, result.Orders[0].FinalReason)
}

func TestEntryRejectionExpiresButExitRejectionIsRetried(t *testing.T) {
	result := runScriptedBars(t, []market.Bar{
		engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000),
		engineSuspendedBar("2026-01-06"),
		engineBar("2026-01-07", 100_000, 100_000, 100_000, 100_000),
	}, []strategy.Action{strategy.EnterLong, strategy.Hold, strategy.Hold}, nil, 0)
	require.Len(t, result.Orders, 1)
	assert.Equal(t, backtest.UnfilledNotTradable, result.Orders[0].FinalReason)
	assert.Empty(t, result.Fills)

	result = runScriptedBars(t, []market.Bar{
		engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000),
		engineBar("2026-01-06", 100_000, 100_000, 100_000, 100_000),
		engineSuspendedBar("2026-01-07"),
		engineBar("2026-01-08", 100_000, 100_000, 100_000, 100_000),
	}, []strategy.Action{strategy.EnterLong, strategy.ExitLong, strategy.Hold, strategy.Hold}, nil, 0)
	require.Len(t, result.Fills, 2)
	require.Len(t, result.Orders, 3)
	assert.Equal(t, backtest.UnfilledNotTradable, result.Orders[1].FinalReason)
	assert.Equal(t, backtest.OrderFilled, result.Orders[2].FinalReason)
	assert.Equal(t, engineOpenTime("2026-01-08"), result.Fills[1].Time)
}

func TestEngineFailsFastForStrategyContextAndErrors(t *testing.T) {
	timeline := engineTimeline(t, []market.Bar{engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000)})
	_, err := (backtest.Engine{}).Run(context.Background(), backtest.Input{
		Strategy: futureStrategy{}, Timeline: timeline, Config: engineConfig(0),
	})
	assert.ErrorIs(t, err, strategy.ErrFutureAccess)

	want := errors.New("strategy failed")
	_, err = (backtest.Engine{}).Run(context.Background(), backtest.Input{
		Strategy: errorStrategy{err: want}, Timeline: timeline, Config: engineConfig(0),
	})
	assert.ErrorIs(t, err, want)
}

func TestEngineReturnsNoPartialResultWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := (backtest.Engine{}).Run(ctx, backtest.Input{
		Strategy: &scriptedStrategy{actions: []strategy.Action{strategy.EnterLong}},
		Timeline: engineTimeline(t, []market.Bar{engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000)}),
		Config:   engineConfig(0),
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, backtest.Result{}, result)
}

func TestEngineRejectsIncompleteInputAndAmbiguousActionTime(t *testing.T) {
	timeline := engineTimeline(t, []market.Bar{engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000)})
	_, err := (backtest.Engine{}).Run(context.Background(), backtest.Input{Timeline: timeline, Config: engineConfig(0)})
	assert.ErrorIs(t, err, backtest.ErrInvalidInput)

	_, err = (backtest.Engine{}).Run(context.Background(), backtest.Input{
		Strategy: &scriptedStrategy{}, Timeline: timeline, Config: engineConfig(0),
		Actions: []market.CorporateAction{{ID: "late", Instrument: engineInstrument(), ExDate: engineCloseTime("2026-01-05"), Kind: market.CashDividend}},
	})
	assert.ErrorIs(t, err, backtest.ErrInvalidInput)
}

type scriptedStrategy struct {
	actions []strategy.Action
	seen    []strategy.PositionView
}

func (s *scriptedStrategy) Definition() strategy.Definition { return engineDefinition() }

func (s *scriptedStrategy) OnBar(ctx strategy.Context) (strategy.Decision, error) {
	s.seen = append(s.seen, ctx.Position())
	if ctx.Index() >= len(s.actions) {
		return strategy.Decision{Action: strategy.Hold}, nil
	}
	return strategy.Decision{Action: s.actions[ctx.Index()], Reason: fmt.Sprintf("bar-%d", ctx.Index())}, nil
}

type futureStrategy struct{}

func (futureStrategy) Definition() strategy.Definition { return engineDefinition() }
func (futureStrategy) OnBar(ctx strategy.Context) (strategy.Decision, error) {
	ctx.Float(indicator.Ref{}, -1)
	return strategy.Decision{Action: strategy.Hold}, nil
}

type errorStrategy struct{ err error }

func (s errorStrategy) Definition() strategy.Definition { return engineDefinition() }
func (s errorStrategy) OnBar(strategy.Context) (strategy.Decision, error) {
	return strategy.Decision{}, s.err
}

func engineDefinition() strategy.Definition {
	return strategy.Definition{ID: "scripted", Version: "1", PrimaryTimeframe: market.Day, DefaultHoldBars: 10}
}

func runScripted(t *testing.T, actions []strategy.Action, corporateActions []market.CorporateAction) backtest.Result {
	t.Helper()
	return runScriptedBars(t, []market.Bar{
		engineBar("2026-01-05", 100_000, 100_000, 100_000, 100_000),
		engineBar("2026-01-06", 100_000, 100_000, 100_000, 100_000),
	}, actions, corporateActions, 0)
}

func runScriptedBars(t *testing.T, bars []market.Bar, actions []strategy.Action, corporateActions []market.CorporateAction, holdBars int) backtest.Result {
	t.Helper()
	result, err := (backtest.Engine{}).Run(context.Background(), backtest.Input{
		Strategy: &scriptedStrategy{actions: actions}, Timeline: engineTimeline(t, bars), Actions: corporateActions, Config: engineConfig(holdBars),
	})
	require.NoError(t, err)
	return result
}

func engineTimeline(t *testing.T, bars []market.Bar) strategy.Timeline {
	t.Helper()
	dataset, err := market.NewDataset(engineInstrument(), market.Day, 1, bars)
	require.NoError(t, err)
	timeline, err := strategy.NewTimeline(dataset, nil, nil)
	require.NoError(t, err)
	return timeline
}

func engineConfig(holdBars int) backtest.Config {
	return backtest.Config{InitialCash: 100_000_000, CashFractionBPS: 10_000, LotSize: 100, HoldBars: holdBars}
}

func engineInstrument() market.InstrumentID {
	return market.InstrumentID{Exchange: market.SSE, Code: "600000"}
}

func engineBar(day string, open, low, high, close market.Price) market.Bar {
	return market.Bar{Instrument: engineInstrument(), Timeframe: market.Day, Version: 1, OpenTime: engineOpenTime(day), CloseTime: engineCloseTime(day), Open: open, Low: low, High: high, Close: close, Volume: 1}
}

func engineSuspendedBar(day string) market.Bar {
	bar := engineBar(day, 100_000, 100_000, 100_000, 100_000)
	bar.Trading = market.Suspended
	bar.Volume = 0
	return bar
}

func engineOpenTime(day string) time.Time  { return engineTime(day, 9) }
func engineCloseTime(day string) time.Time { return engineTime(day, 15) }
func engineTime(day string, hour int) time.Time {
	parsed, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return parsed.UTC().Add(time.Duration(hour) * time.Hour)
}
