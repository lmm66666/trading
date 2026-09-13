package backtest

import (
	"errors"
	"sort"

	"trading/internal/market"
)

var (
	ErrDuplicateFill              = errors.New("backtest: duplicate fill")
	ErrInsufficientCash           = errors.New("backtest: insufficient cash")
	ErrInsufficientPosition       = errors.New("backtest: insufficient position")
	ErrInvalidFill                = errors.New("backtest: invalid fill")
	ErrAccountOverflow            = errors.New("backtest: account transition overflow")
	ErrDuplicateCorporateAction   = errors.New("backtest: duplicate corporate action")
	ErrUnsupportedCorporateAction = errors.New("backtest: unsupported corporate action")
	ErrInvalidCorporateAction     = errors.New("backtest: invalid corporate action")
)

// Position is a value snapshot. Account never returns its mutable internals.
type Position struct {
	Instrument  market.InstrumentID
	Quantity    int64
	CostBasis   market.Money
	AverageCost market.Price
}

// Account is a cash account. Its maps are private and are never exposed.
type Account struct {
	cash             market.Money
	positions        map[market.InstrumentID]Position
	appliedFillIDs   map[string]struct{}
	appliedOrderIDs  map[string]struct{}
	appliedActionIDs map[string]struct{}
}

func NewAccount(initialCash market.Money) (Account, error) {
	if initialCash < 0 {
		return Account{}, ErrInsufficientCash
	}
	return Account{
		cash: initialCash, positions: make(map[market.InstrumentID]Position), appliedFillIDs: make(map[string]struct{}),
		appliedOrderIDs: make(map[string]struct{}), appliedActionIDs: make(map[string]struct{}),
	}, nil
}

func (a Account) Cash() market.Money { return a.cash }

func (a Account) Position(id market.InstrumentID) (Position, bool) {
	position, ok := a.positions[id]
	return position, ok
}

func (a Account) Positions() []Position {
	positions := make([]Position, 0, len(a.positions))
	for _, position := range a.positions {
		positions = append(positions, position)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].Instrument.String() < positions[j].Instrument.String() })
	return positions
}

func (a Account) hasFilledOrder(orderID string) bool {
	_, exists := a.appliedOrderIDs[orderID]
	return exists
}

// CanApplyFill validates the full account transition without mutating a.
func (a Account) CanApplyFill(fill Fill) error {
	_, err := a.previewFill(fill)
	return err
}

func (a *Account) ApplyFill(fill Fill) error {
	next, err := a.previewFill(fill)
	if err != nil {
		return err
	}
	*a = next
	return nil
}

func (a Account) previewFill(fill Fill) (Account, error) {
	if err := validateFill(fill); err != nil {
		return Account{}, err
	}
	if _, exists := a.appliedFillIDs[fill.ID]; exists {
		return Account{}, ErrDuplicateFill
	}
	if _, exists := a.appliedOrderIDs[fill.OrderID]; exists {
		return Account{}, ErrDuplicateFill
	}
	next := Account{
		cash:             a.cash,
		positions:        clonePositions(a.positions),
		appliedFillIDs:   cloneIDs(a.appliedFillIDs),
		appliedOrderIDs:  cloneIDs(a.appliedOrderIDs),
		appliedActionIDs: cloneIDs(a.appliedActionIDs),
	}
	position := next.positions[fill.Instrument]
	switch fill.Side {
	case Buy:
		debit, ok := fill.TotalDebit()
		if !ok {
			return Account{}, ErrInvalidFill
		}
		if next.cash < debit {
			return Account{}, ErrInsufficientCash
		}
		costBasis, ok := addMoney(position.CostBasis, debit)
		if !ok {
			return Account{}, ErrAccountOverflow
		}
		quantity, ok := addInt64(position.Quantity, fill.Quantity)
		if !ok {
			return Account{}, ErrAccountOverflow
		}
		position = makePosition(fill.Instrument, quantity, costBasis)
		next.cash -= debit
		next.positions[fill.Instrument] = position
	case Sell:
		if position.Quantity < fill.Quantity {
			return Account{}, ErrInsufficientPosition
		}
		credit, ok := fill.NetCredit()
		if !ok {
			return Account{}, ErrInvalidFill
		}
		remaining := position.Quantity - fill.Quantity
		var remainingBasis market.Money
		if remaining > 0 {
			basis, ok := mulDivFloor(int64(position.CostBasis), remaining, position.Quantity)
			if !ok {
				return Account{}, ErrAccountOverflow
			}
			remainingBasis = market.Money(basis)
		}
		cash, ok := addMoney(next.cash, credit)
		if !ok {
			return Account{}, ErrAccountOverflow
		}
		next.cash = cash
		if remaining == 0 {
			delete(next.positions, fill.Instrument)
		} else {
			next.positions[fill.Instrument] = makePosition(fill.Instrument, remaining, remainingBasis)
		}
	default:
		return Account{}, ErrInvalidFill
	}
	next.appliedFillIDs[fill.ID] = struct{}{}
	next.appliedOrderIDs[fill.OrderID] = struct{}{}
	return next, nil
}

// ApplyCorporateActions validates the complete batch before atomically making
// any cash, position, or idempotency state visible.
func (a *Account) ApplyCorporateActions(actions []market.CorporateAction) error {
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if err := validateCorporateAction(action); err != nil {
			return err
		}
		if _, exists := seen[action.ID]; exists {
			return ErrDuplicateCorporateAction
		}
		seen[action.ID] = struct{}{}
		if _, exists := a.appliedActionIDs[action.ID]; exists {
			return ErrDuplicateCorporateAction
		}
	}

	cash := a.cash
	positions := clonePositions(a.positions)
	applied := cloneIDs(a.appliedActionIDs)
	for _, action := range actions {
		position, exists := positions[action.Instrument]
		if exists {
			switch action.Kind {
			case market.CashDividend:
				dividend, ok := mulDivFloor(position.Quantity, int64(action.CashPerShare), 1)
				if !ok {
					return ErrInvalidCorporateAction
				}
				newCash, ok := addMoney(cash, market.Money(dividend))
				if !ok {
					return ErrInvalidCorporateAction
				}
				cash = newCash
			case market.ShareDistribution:
				quantity, ok := mulDivFloor(position.Quantity, action.ShareNumerator, action.ShareDenominator)
				if !ok || quantity <= 0 {
					return ErrInvalidCorporateAction
				}
				positions[action.Instrument] = makePosition(action.Instrument, quantity, position.CostBasis)
			}
		}
		applied[action.ID] = struct{}{}
	}
	a.cash, a.positions, a.appliedActionIDs = cash, positions, applied
	return nil
}

func validateFill(fill Fill) error {
	if fill.ID == "" || fill.OrderID == "" || fill.Instrument.Validate() != nil || fill.Time.IsZero() || fill.Price <= 0 || fill.Quantity <= 0 ||
		(fill.Side != Buy && fill.Side != Sell) || fill.Commission < 0 || fill.StampDuty < 0 || fill.TransferFee < 0 {
		return ErrInvalidFill
	}
	gross, ok := mulDivFloor(int64(fill.Price), fill.Quantity, 1)
	if !ok || fill.Gross != market.Money(gross) {
		return ErrInvalidFill
	}
	if fill.Side == Buy {
		_, ok = fill.TotalDebit()
	} else {
		_, ok = fill.NetCredit()
	}
	if !ok {
		return ErrInvalidFill
	}
	return nil
}

func validateCorporateAction(action market.CorporateAction) error {
	if action.ID == "" || action.Instrument.Validate() != nil {
		return ErrInvalidCorporateAction
	}
	switch action.Kind {
	case market.CashDividend:
		if action.CashPerShare < 0 {
			return ErrInvalidCorporateAction
		}
	case market.ShareDistribution:
		if action.ShareNumerator <= 0 || action.ShareDenominator <= 0 {
			return ErrInvalidCorporateAction
		}
	case market.RightsIssue:
		return ErrUnsupportedCorporateAction
	default:
		return ErrInvalidCorporateAction
	}
	return nil
}

func makePosition(id market.InstrumentID, quantity int64, costBasis market.Money) Position {
	average, _ := mulDivFloor(int64(costBasis), 1, quantity)
	return Position{Instrument: id, Quantity: quantity, CostBasis: costBasis, AverageCost: market.Price(average)}
}

func clonePositions(input map[market.InstrumentID]Position) map[market.InstrumentID]Position {
	output := make(map[market.InstrumentID]Position, len(input))
	for id, position := range input {
		output[id] = position
	}
	return output
}

func cloneIDs(input map[string]struct{}) map[string]struct{} {
	output := make(map[string]struct{}, len(input))
	for id := range input {
		output[id] = struct{}{}
	}
	return output
}

func addInt64(left, right int64) (int64, bool) {
	if right < 0 || left > int64(^uint64(0)>>1)-right {
		return 0, false
	}
	return left + right, true
}
