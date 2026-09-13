package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

type instrumentCatalogStub struct {
	items  []port.InstrumentSummary
	item   port.InstrumentSummary
	search port.InstrumentSearch
	err    error
}

func (s *instrumentCatalogStub) Search(_ context.Context, search port.InstrumentSearch) ([]port.InstrumentSummary, error) {
	s.search = search
	return append([]port.InstrumentSummary(nil), s.items...), s.err
}

func (s *instrumentCatalogStub) Get(_ context.Context, _ market.InstrumentID) (port.InstrumentSummary, error) {
	return s.item, s.err
}

func TestInstrumentSearchNormalizesAndDelegates(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SZSE, Code: "002415"}
	catalog := &instrumentCatalogStub{items: []port.InstrumentSummary{{ID: id, Name: "海康威视", Active: true, LotSize: 100}}}
	service, err := application.NewInstrumentQueryService(catalog)
	require.NoError(t, err)

	got, err := service.Search(context.Background(), application.InstrumentSearchQuery{Query: "  海康  ", Exchange: market.SZSE})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "海康威视", got[0].Name)
	assert.Equal(t, port.InstrumentSearch{Query: "海康", Exchange: market.SZSE, Limit: 20}, catalog.search)
}

func TestInstrumentSearchRejectsInvalidBounds(t *testing.T) {
	service, err := application.NewInstrumentQueryService(&instrumentCatalogStub{})
	require.NoError(t, err)

	for _, query := range []application.InstrumentSearchQuery{
		{},
		{Query: "600000", Limit: 51},
		{Query: "600000", Exchange: market.Exchange("NASDAQ")},
		{Query: string(make([]byte, 129))},
	} {
		_, err := service.Search(context.Background(), query)
		assert.ErrorIs(t, err, application.ErrInvalidRequest)
	}
}

func TestInstrumentGetRejectsMismatchedStoredIdentity(t *testing.T) {
	want := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	catalog := &instrumentCatalogStub{item: port.InstrumentSummary{ID: market.InstrumentID{Exchange: market.SZSE, Code: "000001"}, Active: true}}
	service, err := application.NewInstrumentQueryService(catalog)
	require.NoError(t, err)

	_, err = service.Get(context.Background(), want)
	assert.ErrorIs(t, err, application.ErrIncompleteMarketData)
}
