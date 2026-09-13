package port

import (
	"context"

	"trading/internal/market"
)

const (
	DefaultInstrumentSearchLimit = 20
	MaxInstrumentSearchLimit     = 50
	MaxInstrumentSearchBytes     = 128
)

type InstrumentSearch struct {
	Query    string
	Exchange market.Exchange
	Limit    int
}

type InstrumentSummary struct {
	ID      market.InstrumentID
	Name    string
	Board   string
	Active  bool
	LotSize int64
}

type InstrumentCatalog interface {
	Search(context.Context, InstrumentSearch) ([]InstrumentSummary, error)
	Get(context.Context, market.InstrumentID) (InstrumentSummary, error)
}
