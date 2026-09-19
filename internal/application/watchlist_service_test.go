package application

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
	"trading/internal/port"
)

type watchlistStoreFake struct {
	entries []port.WatchlistEntry
	added   []market.InstrumentID
	removed []market.InstrumentID
	count   int
	listErr error
	addErr  error
}

func (s *watchlistStoreFake) List(context.Context) ([]port.WatchlistEntry, error) {
	return s.entries, s.listErr
}
func (s *watchlistStoreFake) Add(_ context.Context, id market.InstrumentID) (bool, error) {
	s.added = append(s.added, id)
	return false, s.addErr
}
func (s *watchlistStoreFake) Remove(_ context.Context, id market.InstrumentID) error {
	s.removed = append(s.removed, id)
	return nil
}
func (s *watchlistStoreFake) Count(context.Context) (int, error) {
	return s.count, nil
}

type watchlistQuotesFake struct {
	ids    []uint64
	quotes map[uint64]port.DailyQuote
	err    error
}

func (q *watchlistQuotesFake) LatestDailyQuotes(_ context.Context, ids []uint64) (map[uint64]port.DailyQuote, error) {
	q.ids = ids
	return q.quotes, q.err
}

type watchlistCatalogFake struct {
	get   func(market.InstrumentID) (port.InstrumentSummary, error)
	seen  market.InstrumentID
	calls int
}

func (c *watchlistCatalogFake) Search(context.Context, port.InstrumentSearch) ([]port.InstrumentSummary, error) {
	return nil, nil
}
func (c *watchlistCatalogFake) Get(_ context.Context, id market.InstrumentID) (port.InstrumentSummary, error) {
	c.seen = id
	c.calls++
	if c.get != nil {
		return c.get(id)
	}
	return port.InstrumentSummary{ID: id, Active: true}, nil
}

func newWatchlistFixture(t *testing.T) (*WatchlistService, *watchlistStoreFake, *watchlistQuotesFake, *watchlistCatalogFake) {
	t.Helper()
	store := &watchlistStoreFake{}
	quotes := &watchlistQuotesFake{}
	catalog := &watchlistCatalogFake{}
	service, err := NewWatchlistService(store, quotes, catalog)
	require.NoError(t, err)
	return service, store, quotes, catalog
}

func TestNewWatchlistServiceRequiresDependencies(t *testing.T) {
	store := &watchlistStoreFake{}
	quotes := &watchlistQuotesFake{}
	catalog := &watchlistCatalogFake{}
	for _, tt := range []struct {
		name    string
		store   port.WatchlistStore
		quotes  port.DailyQuoteReader
		catalog port.InstrumentCatalog
	}{{"nil store", nil, quotes, catalog}, {"nil quotes", store, nil, catalog}, {"nil catalog", store, quotes, nil}} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWatchlistService(tt.store, tt.quotes, tt.catalog)
			require.ErrorIs(t, err, ErrInvalidRequest)
		})
	}
}

func TestWatchlistListEnrichesQuotes(t *testing.T) {
	service, store, quotes, _ := newWatchlistFixture(t)
	pfu := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	citic := market.InstrumentID{Exchange: market.SZSE, Code: "000001"}
	store.entries = []port.WatchlistEntry{
		{InstrumentSummary: port.InstrumentSummary{ID: pfu, Name: "浦发银行", Board: "MAIN", Active: true, LotSize: 100}, InstrumentRowID: 11},
		{InstrumentSummary: port.InstrumentSummary{ID: citic, Name: "中信证券", Board: "MAIN", Active: true, LotSize: 100}, InstrumentRowID: 22},
	}
	close := 12.34
	change := 0.21
	pct := 1.73
	quotes.quotes = map[uint64]port.DailyQuote{11: {Close: &close, Change: &change, ChangePercent: &pct}, 22: {Close: &close}}

	items, err := service.List(context.Background())
	require.NoError(t, err)
	require.Equal(t, []uint64{11, 22}, quotes.ids)
	require.Len(t, items, 2)
	require.Equal(t, pfu, items[0].ID)
	require.Equal(t, "浦发银行", items[0].Name)
	require.NotNil(t, items[0].Close)
	require.InDelta(t, 12.34, *items[0].Close, 1e-9)
	require.NotNil(t, items[0].Change)
	require.InDelta(t, 0.21, *items[0].Change, 1e-9)
	require.NotNil(t, items[0].ChangePercent)
	require.InDelta(t, 1.73, *items[0].ChangePercent, 1e-9)
	require.Equal(t, citic, items[1].ID)
	require.NotNil(t, items[1].Close)
	require.Nil(t, items[1].Change)
	require.Nil(t, items[1].ChangePercent)
}

func TestWatchlistListLeavesItemsWithoutQuotesNull(t *testing.T) {
	service, store, quotes, _ := newWatchlistFixture(t)
	store.entries = []port.WatchlistEntry{{InstrumentSummary: port.InstrumentSummary{ID: market.InstrumentID{Exchange: market.SSE, Code: "600000"}, Active: true}, InstrumentRowID: 33}}
	quotes.quotes = map[uint64]port.DailyQuote{}

	items, err := service.List(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].Close)
	require.Nil(t, items[0].Change)
	require.Nil(t, items[0].ChangePercent)
}

func TestWatchlistListPropagatesFailures(t *testing.T) {
	service, store, _, _ := newWatchlistFixture(t)
	store.listErr = errors.New("store down")
	_, err := service.List(context.Background())
	require.ErrorIs(t, err, store.listErr)

	service, store, quotes, _ := newWatchlistFixture(t)
	store.entries = []port.WatchlistEntry{{InstrumentRowID: 1}}
	quotes.err = errors.New("quotes down")
	_, err = service.List(context.Background())
	require.ErrorIs(t, err, quotes.err)
}

func TestWatchlistAddRejectsInvalidInstrument(t *testing.T) {
	service, _, _, catalog := newWatchlistFixture(t)
	_, err := service.Add(context.Background(), market.InstrumentID{Exchange: market.Exchange("NASDAQ"), Code: "AAPL"})
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.Zero(t, catalog.calls)
}

func TestWatchlistAddRequiresKnownActiveInstrument(t *testing.T) {
	service, _, _, catalog := newWatchlistFixture(t)
	notFound := errors.New("unknown instrument")
	catalog.get = func(id market.InstrumentID) (port.InstrumentSummary, error) {
		return port.InstrumentSummary{}, notFound
	}
	_, err := service.Add(context.Background(), market.InstrumentID{Exchange: market.SSE, Code: "600000"})
	require.ErrorIs(t, err, notFound)

	catalog.get = func(id market.InstrumentID) (port.InstrumentSummary, error) {
		return port.InstrumentSummary{ID: id, Active: false}, nil
	}
	_, err = service.Add(context.Background(), market.InstrumentID{Exchange: market.SSE, Code: "600000"})
	require.ErrorIs(t, err, port.ErrMarketDataNotFound)
}

func TestWatchlistAddRejectsWhenFull(t *testing.T) {
	service, store, _, _ := newWatchlistFixture(t)
	store.count = port.MaxWatchlistItems
	_, err := service.Add(context.Background(), market.InstrumentID{Exchange: market.SSE, Code: "600000"})
	require.ErrorIs(t, err, ErrWatchlistFull)
	require.Empty(t, store.added)
}

func TestWatchlistAddPersistsAndReturnsUpdatedList(t *testing.T) {
	service, store, _, _ := newWatchlistFixture(t)
	pfu := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	store.entries = []port.WatchlistEntry{{InstrumentSummary: port.InstrumentSummary{ID: pfu, Name: "浦发银行", Active: true}, InstrumentRowID: 9}}

	items, err := service.Add(context.Background(), pfu)
	require.NoError(t, err)
	require.Equal(t, []market.InstrumentID{pfu}, store.added)
	require.Len(t, items, 1)
	require.Equal(t, pfu, items[0].ID)
	require.Nil(t, items[0].Close)
}

func TestWatchlistRemoveRejectsInvalidInstrumentAndPropagatesStoreFailure(t *testing.T) {
	service, _, _, _ := newWatchlistFixture(t)
	_, err := service.Remove(context.Background(), market.InstrumentID{Exchange: market.Exchange("NASDAQ"), Code: "AAPL"})
	require.ErrorIs(t, err, ErrInvalidRequest)

	service, store, _, _ := newWatchlistFixture(t)
	store.listErr = errors.New("store down")
	pfu := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	_, err = service.Remove(context.Background(), pfu)
	require.ErrorIs(t, err, store.listErr)
	require.Equal(t, []market.InstrumentID{pfu}, store.removed)
}

func TestWatchlistRemoveReturnsUpdatedList(t *testing.T) {
	service, store, _, _ := newWatchlistFixture(t)
	pfu := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	close := 12.34
	store.entries = []port.WatchlistEntry{{InstrumentSummary: port.InstrumentSummary{ID: pfu, Name: "浦发银行", Active: true}, InstrumentRowID: 9}}
	quotes := &watchlistQuotesFake{quotes: map[uint64]port.DailyQuote{9: {Close: &close}}}
	service.quotes = quotes

	items, err := service.Remove(context.Background(), pfu)
	require.NoError(t, err)
	require.Equal(t, []market.InstrumentID{pfu}, store.removed)
	require.Len(t, items, 1)
	require.InDelta(t, 12.34, *items[0].Close, 1e-9)
}
