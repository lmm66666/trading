package market

import "time"

type CorporateActionKind uint8

const (
	CashDividend CorporateActionKind = iota + 1
	ShareDistribution
	RightsIssue
)

type CorporateAction struct {
	ID               string
	Instrument       InstrumentID
	ExDate           time.Time
	Kind             CorporateActionKind
	CashPerShare     Money
	ShareNumerator   int64
	ShareDenominator int64
	Version          DataVersion
}
