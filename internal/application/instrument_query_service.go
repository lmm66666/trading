package application

import (
	"context"
	"strings"

	"trading/internal/market"
	"trading/internal/port"
)

type InstrumentSearchQuery struct {
	Query    string
	Exchange market.Exchange
	Limit    int
}

type InstrumentQueryService struct {
	catalog port.InstrumentCatalog
}

func NewInstrumentQueryService(catalog port.InstrumentCatalog) (*InstrumentQueryService, error) {
	if catalog == nil {
		return nil, invalidRequest("instrument catalog is required")
	}
	return &InstrumentQueryService{catalog: catalog}, nil
}

func (s *InstrumentQueryService) Search(ctx context.Context, query InstrumentSearchQuery) ([]port.InstrumentSummary, error) {
	query.Query = strings.TrimSpace(query.Query)
	if query.Query == "" || len([]byte(query.Query)) > port.MaxInstrumentSearchBytes {
		return nil, invalidRequest("instrument search query is invalid")
	}
	if query.Exchange != "" && query.Exchange != market.SSE && query.Exchange != market.SZSE && query.Exchange != market.BSE {
		return nil, invalidRequest("instrument search exchange is invalid")
	}
	if query.Limit == 0 {
		query.Limit = port.DefaultInstrumentSearchLimit
	}
	if query.Limit < 1 || query.Limit > port.MaxInstrumentSearchLimit {
		return nil, invalidRequest("instrument search limit is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := s.catalog.Search(ctx, port.InstrumentSearch{Query: query.Query, Exchange: query.Exchange, Limit: query.Limit})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID.Validate() != nil || !item.Active {
			return nil, ErrIncompleteMarketData
		}
	}
	return items, nil
}

func (s *InstrumentQueryService) Get(ctx context.Context, id market.InstrumentID) (port.InstrumentSummary, error) {
	if err := id.Validate(); err != nil {
		return port.InstrumentSummary{}, invalidRequest("instrument is invalid")
	}
	if err := ctx.Err(); err != nil {
		return port.InstrumentSummary{}, err
	}
	item, err := s.catalog.Get(ctx, id)
	if err != nil {
		return port.InstrumentSummary{}, err
	}
	if item.ID != id || item.ID.Validate() != nil || !item.Active {
		return port.InstrumentSummary{}, ErrIncompleteMarketData
	}
	return item, nil
}
