package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

type apiWatchlist struct {
	items                       []application.WatchlistItem
	addedID, removedID          market.InstrumentID
	addCalls, removeCalls      int
	listErr, addErr, removeErr error
}

func (w *apiWatchlist) List(context.Context) ([]application.WatchlistItem, error) {
	return w.items, w.listErr
}
func (w *apiWatchlist) Add(_ context.Context, id market.InstrumentID) ([]application.WatchlistItem, error) {
	w.addCalls++
	w.addedID = id
	return w.items, w.addErr
}
func (w *apiWatchlist) Remove(_ context.Context, id market.InstrumentID) ([]application.WatchlistItem, error) {
	w.removeCalls++
	w.removedID = id
	return w.items, w.removeErr
}

func watchlistFixtureItems() []application.WatchlistItem {
	close := 12.34
	change := 0.21
	pct := 1.73
	return []application.WatchlistItem{
		{InstrumentSummary: port.InstrumentSummary{ID: market.InstrumentID{Exchange: market.SSE, Code: "600000"}, Name: "浦发银行", Board: "MAIN", Active: true, LotSize: 100}, Close: &close, Change: &change, ChangePercent: &pct},
		{InstrumentSummary: port.InstrumentSummary{ID: market.InstrumentID{Exchange: market.SZSE, Code: "000001"}, Name: "平安银行", Board: "MAIN", Active: true, LotSize: 100}},
	}
}

func TestGetWatchlistReturnsItemsWithNullQuotes(t *testing.T) {
	f := newKernelFixture(t)
	f.services.Watchlist = &apiWatchlist{items: watchlistFixtureItems()}
	f.router = NewRouter(f.services)

	w := kernelRequest(t, f, "GET", "/api/v1/watchlist", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.JSONEq(t, `{"code":0,"message":"success","data":{"items":[
		{"instrument":"SSE:600000","code":"600000","name":"浦发银行","exchange":"SSE","board":"MAIN","lot_size":100,"close":12.34,"change":0.21,"change_pct":1.73},
		{"instrument":"SZSE:000001","code":"000001","name":"平安银行","exchange":"SZSE","board":"MAIN","lot_size":100,"close":null,"change":null,"change_pct":null}
	]}}`, w.Body.String())
}

func TestGetWatchlistMapsErrors(t *testing.T) {
	f := newKernelFixture(t)
	f.services.Watchlist = &apiWatchlist{listErr: internalAPIError}
	f.router = NewRouter(f.services)
	w := kernelRequest(t, f, "GET", "/api/v1/watchlist", "")
	require.Equal(t, 500, w.Code)
	require.Contains(t, w.Body.String(), "internal server error")
	require.NotContains(t, w.Body.String(), "secret")

	f.services.Watchlist = nil
	f.router = NewRouter(f.services)
	w = kernelRequest(t, f, "GET", "/api/v1/watchlist", "")
	require.Equal(t, 500, w.Code)
}
