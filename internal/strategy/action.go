package strategy

// Action is the intent emitted by a strategy after a bar closes.
type Action uint8

const (
	Hold Action = iota
	EnterLong
	ExitLong
)

// Decision describes a strategy intent without mutating an account.
type Decision struct {
	Action Action
	Reason string
	Values map[string]float64
}
