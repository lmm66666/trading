package backtest

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"trading/internal/market"
	"trading/internal/strategy"
)

var ErrInvalidInput = errors.New("backtest: invalid engine input")

// Input is a complete, single-instrument backtest request. Timeline and
// actions are checked before any strategy or account mutation occurs.
type Input struct {
	Strategy strategy.Strategy
	Timeline strategy.Timeline
	Actions  []market.CorporateAction
	Config   Config
}

// Engine executes a strategy serially. It has no mutable fields, so one value
// can be reused safely as long as each Run receives independent inputs.
type Engine struct{}

type engineState struct {
	input        Input
	timeline     strategy.Timeline
	definition   strategy.Definition
	account      Account
	execution    ExecutionModel
	actionsAt    map[int][]market.CorporateAction
	orders       []Order
	fills        []Fill
	trades       []Trade
	equity       []EquityPoint
	pendingOrder int
	entry        *entryState
	exitRetry    bool
	holdBars     int
}

type entryState struct {
	fill     Fill
	barIndex int
}

// Run advances exactly one confirmed Bar at a time: apply actions before the
// open, attempt the previous close's pending order once, mark close equity,
// execute strategy code, then queue the next-open order.
func (Engine) Run(ctx context.Context, input Input) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("%w: nil context", ErrInvalidInput)
	}
	state, err := newEngineState(input)
	if err != nil {
		return Result{}, err
	}
	for index := 0; index < state.timeline.Len(); index++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err := state.applyActionsBeforeOpen(ctx, index); err != nil {
			return Result{}, err
		}
		if err := state.tryPendingOrderAtOpen(ctx, index); err != nil {
			return Result{}, err
		}
		if err := state.markToMarketAtClose(ctx, index); err != nil {
			return Result{}, err
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		position := state.positionView(index)
		barContext := newBarContext(state.timeline, index, position)
		decision, err := state.input.Strategy.OnBar(barContext)
		if contextErr := barContext.Err(); contextErr != nil {
			return Result{}, contextErr
		}
		if err != nil {
			return Result{}, fmt.Errorf("strategy bar %d: %w", index, err)
		}
		if decision.Action < strategy.Hold || decision.Action > strategy.ExitLong {
			return Result{}, fmt.Errorf("%w: strategy bar %d action", ErrInvalidInput, index)
		}
		if err := state.queueForNextOpen(ctx, index, decision); err != nil {
			return Result{}, err
		}
	}
	position := state.positionView(state.timeline.Len() - 1)
	result := Result{Orders: state.orders, Fills: state.fills, Trades: state.trades, Equity: state.equity, FinalPosition: position}
	result.Summary = calculateMetricsWithInitial(result.Equity, result.Trades, position, input.Config.InitialCash)
	return result, nil
}

func newEngineState(input Input) (*engineState, error) {
	if isNilStrategy(input.Strategy) {
		return nil, fmt.Errorf("%w: strategy", ErrInvalidInput)
	}
	if err := input.Config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: config: %w", ErrInvalidInput, err)
	}
	timeline, err := strategy.NewTimeline(input.Timeline.Primary, input.Timeline.Features, input.Timeline.Auxiliary)
	if err != nil {
		return nil, fmt.Errorf("%w: timeline: %w", ErrInvalidInput, err)
	}
	if timeline.Len() == 0 {
		return nil, fmt.Errorf("%w: empty timeline", ErrInvalidInput)
	}
	definition := input.Strategy.Definition()
	if err := validateDefinition(definition, timeline); err != nil {
		return nil, err
	}
	actionsAt, err := indexActions(input.Actions, timeline)
	if err != nil {
		return nil, err
	}
	account, err := NewAccount(input.Config.InitialCash)
	if err != nil {
		return nil, fmt.Errorf("%w: account: %w", ErrInvalidInput, err)
	}
	execution, err := NewExecutionModel(input.Config)
	if err != nil {
		return nil, fmt.Errorf("%w: execution: %w", ErrInvalidInput, err)
	}
	holdBars := input.Config.HoldBars
	if holdBars == 0 {
		holdBars = definition.DefaultHoldBars
	}
	if holdBars == 0 {
		holdBars = 10
	}
	return &engineState{input: input, timeline: timeline, definition: definition, account: account, execution: execution, actionsAt: actionsAt, pendingOrder: -1, holdBars: holdBars}, nil
}

func validateDefinition(definition strategy.Definition, timeline strategy.Timeline) error {
	if definition.ID == "" || definition.Version == "" || !definition.PrimaryTimeframe.Valid() || definition.PrimaryTimeframe != timeline.Primary.Timeframe() || definition.WarmupBars < 0 || definition.DefaultHoldBars < 0 {
		return fmt.Errorf("%w: strategy definition", ErrInvalidInput)
	}
	for _, ref := range definition.Features {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("%w: strategy feature", ErrInvalidInput)
		}
		if ref.Timeframe == timeline.Primary.Timeframe() {
			if _, ok := timeline.Features[ref.Key()]; !ok {
				return fmt.Errorf("%w: missing strategy feature", ErrInvalidInput)
			}
			continue
		}
		aligned, ok := timeline.Auxiliary[ref.Timeframe]
		if !ok {
			return fmt.Errorf("%w: missing auxiliary feature", ErrInvalidInput)
		}
		if _, ok := aligned.Features[ref.Key()]; !ok {
			return fmt.Errorf("%w: missing strategy feature", ErrInvalidInput)
		}
	}
	return nil
}

func indexActions(actions []market.CorporateAction, timeline strategy.Timeline) (map[int][]market.CorporateAction, error) {
	indexed := make(map[int][]market.CorporateAction)
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if action.ID == "" || action.Instrument != timeline.Primary.Instrument() || action.ExDate.IsZero() || action.Version != timeline.Primary.Version() {
			return nil, fmt.Errorf("%w: corporate action ownership or version", ErrInvalidInput)
		}
		if _, exists := seen[action.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate corporate action", ErrInvalidInput)
		}
		seen[action.ID] = struct{}{}
		index := -1
		for i := 0; i < timeline.Len(); i++ {
			if action.ExDate.Equal(timeline.Primary.Bar(i).OpenTime) {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("%w: corporate action time", ErrInvalidInput)
		}
		indexed[index] = append(indexed[index], action)
	}
	return indexed, nil
}

func (s *engineState) applyActionsBeforeOpen(ctx context.Context, index int) error {
	actions := s.actionsAt[index]
	if len(actions) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.account.ApplyCorporateActions(actions); err != nil {
		return fmt.Errorf("apply corporate actions bar %d: %w", index, err)
	}
	return nil
}

func (s *engineState) tryPendingOrderAtOpen(ctx context.Context, index int) error {
	if s.pendingOrder < 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	orderIndex := s.pendingOrder
	order := s.orders[orderIndex]
	bar := s.timeline.Primary.Bar(index)
	fill, reject := s.execution.Execute(order, bar, s.account)
	if err := ctx.Err(); err != nil {
		return err
	}
	s.orders[orderIndex].AttemptedAt = bar.OpenTime
	s.pendingOrder = -1
	if reject != RejectNone {
		s.orders[orderIndex].FinalReason = finalReason(reject)
		if order.Side == Sell {
			s.exitRetry = true
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.account.ApplyFill(fill); err != nil {
		return fmt.Errorf("apply fill bar %d: %w", index, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.orders[orderIndex].FinalReason = OrderFilled
	s.fills = append(s.fills, fill)
	if fill.Side == Buy {
		s.entry = &entryState{fill: fill, barIndex: index}
		return nil
	}
	if s.entry == nil {
		return fmt.Errorf("%w: sell without entry", ErrInvalidInput)
	}
	trade, err := newTrade(s.entry.fill, fill, index-s.entry.barIndex)
	if err != nil {
		return err
	}
	s.trades = append(s.trades, trade)
	s.entry = nil
	s.exitRetry = false
	return nil
}

func (s *engineState) markToMarketAtClose(ctx context.Context, index int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bar := s.timeline.Primary.Bar(index)
	positionValue, err := s.positionValue(bar.Close)
	if err != nil {
		return fmt.Errorf("mark to market bar %d: %w", index, err)
	}
	equity, ok := addMoney(s.account.Cash(), positionValue)
	if !ok {
		return fmt.Errorf("mark to market bar %d: %w", index, ErrAccountOverflow)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.equity = append(s.equity, EquityPoint{Time: bar.CloseTime, Equity: equity, Cash: s.account.Cash(), PositionValue: positionValue})
	return nil
}

func (s *engineState) queueForNextOpen(ctx context.Context, index int, decision strategy.Decision) error {
	position := s.positionView(index)
	side := Side(0)
	reason := decision.Reason
	if position.Open && (decision.Action == strategy.ExitLong || s.exitRetry || position.HoldingBars >= s.holdBars) {
		side = Sell
		if reason == "" {
			reason = "default_hold_exit"
		}
	} else if !position.Open && decision.Action == strategy.EnterLong {
		side = Buy
	}
	if side == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	order := NewNextOpenOrder(side, s.timeline.Primary.Bar(index).CloseTime, reason)
	order.Instrument = s.timeline.Primary.Instrument()
	if index == s.timeline.Len()-1 {
		order.FinalReason = UnfilledNoNextBar
		s.orders = append(s.orders, order)
		return nil
	}
	order.FinalReason = OrderPending
	s.orders = append(s.orders, order)
	s.pendingOrder = len(s.orders) - 1
	return nil
}

func (s *engineState) positionView(index int) strategy.PositionView {
	position, ok := s.account.Position(s.timeline.Primary.Instrument())
	if !ok || position.Quantity <= 0 || s.entry == nil {
		return strategy.PositionView{}
	}
	return strategy.PositionView{Open: true, Quantity: position.Quantity, HoldingBars: index - s.entry.barIndex + 1}
}

func (s *engineState) positionValue(close market.Price) (market.Money, error) {
	position, ok := s.account.Position(s.timeline.Primary.Instrument())
	if !ok || position.Quantity == 0 {
		return 0, nil
	}
	value, ok := mulDivFloor(position.Quantity, int64(close), 1)
	if !ok {
		return 0, ErrAccountOverflow
	}
	return market.Money(value), nil
}

func newTrade(entry, exit Fill, holdingBars int) (Trade, error) {
	if holdingBars <= 0 || entry.Side != Buy || exit.Side != Sell {
		return Trade{}, fmt.Errorf("%w: invalid round trip", ErrInvalidInput)
	}
	debit, ok := entry.TotalDebit()
	if !ok {
		return Trade{}, ErrAccountOverflow
	}
	credit, ok := exit.NetCredit()
	if !ok {
		return Trade{}, ErrAccountOverflow
	}
	profit, ok := subtractMoney(credit, debit)
	if !ok {
		return Trade{}, ErrAccountOverflow
	}
	return Trade{Entry: entry, Exit: exit, HoldingBars: holdingBars, NetProfit: profit}, nil
}

func subtractMoney(left, right market.Money) (market.Money, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	if left >= right {
		return left - right, true
	}
	return -market.Money(int64(right - left)), true
}

func finalReason(reject RejectReason) OrderFinalReason {
	switch reject {
	case RejectNone:
		return OrderFilled
	case RejectInvalidConfig:
		return UnfilledInvalidConfig
	case RejectInvalidOrder:
		return UnfilledInvalidOrder
	case RejectNotYetActive:
		return UnfilledNotYetActive
	case RejectNotTradable:
		return UnfilledNotTradable
	case RejectLimitUp:
		return UnfilledLimitUp
	case RejectLimitDown:
		return UnfilledLimitDown
	case RejectInvalidLot:
		return UnfilledInvalidLot
	case RejectInsufficientCash:
		return UnfilledInsufficientCash
	case RejectInsufficientPosition:
		return UnfilledInsufficientPosition
	case RejectDuplicateFill:
		return UnfilledDuplicateFill
	case RejectArithmeticOverflow:
		return UnfilledArithmeticOverflow
	case RejectInvalidBar:
		return UnfilledInvalidBar
	case RejectFeesExceedProceeds:
		return UnfilledFeesExceedProceeds
	case RejectAccountOverflow:
		return UnfilledAccountOverflow
	default:
		return UnfilledInvalidAccountTransition
	}
}

func isNilStrategy(value strategy.Strategy) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Ptr && reflected.IsNil()
}
