package strategy

import (
	"errors"

	"trading/internal/indicator"
	"trading/internal/market"
)

var (
	ErrFutureAccess      = errors.New("strategy: attempted future access")
	ErrUnknownStrategy   = errors.New("strategy: unknown strategy")
	ErrUnknownParameter  = errors.New("strategy: unknown parameter")
	ErrDuplicateStrategy = errors.New("strategy: duplicate strategy")
	ErrInvalidParameter  = errors.New("strategy: invalid parameter")
	ErrInvalidDefinition = errors.New("strategy: invalid definition")
	ErrInvalidTimeline   = errors.New("strategy: invalid timeline")
	ErrMissingFeature    = errors.New("strategy: missing feature")
	ErrIncompatibleData  = errors.New("strategy: incompatible timeline")
)

// PositionView is the read-only position information visible to a strategy.
type PositionView struct {
	Open        bool
	Quantity    int64
	HoldingBars int
}

// Context exposes only confirmed data at the current primary bar.
type Context interface {
	Bar() market.Bar
	Index() int
	Float(ref indicator.Ref, ago int) (float64, bool)
	Position() PositionView
	Err() error
}

// Strategy is a compiled, stateful strategy instance for one instrument run.
type Strategy interface {
	Definition() Definition
	OnBar(Context) (Decision, error)
}
