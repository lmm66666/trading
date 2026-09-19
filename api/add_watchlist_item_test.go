package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

func TestAddWatchlistItemPersistsAndReturnsList(t *testing.T) {
	f := newKernelFixture(t)
	watchlist := &apiWatchlist{items: watchlistFixtureItems()}
	f.services.Watchlist = watchlist
	f.router = NewRouter(f.services)

	w := kernelRequest(t, f, "POST", "/api/v1/watchlist", `{"instrument":"SSE:600000"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 1, watchlist.addCalls)
	require.Equal(t, market.InstrumentID{Exchange: market.SSE, Code: "600000"}, watchlist.addedID)
	require.Contains(t, w.Body.String(), `"items":[`)
}

func TestAddWatchlistItemRejectsInvalidBodies(t *testing.T) {
	for _, body := range []string{
		``,
		`{"instrument":""}`,
		`{"instrument":"600000"}`,
		`{"instrument":"NASDAQ:AAPL"}`,
		`{"instrument":"SSE:600000","extra":1}`,
		`{"instrument":"SSE:600000"}{"instrument":"SSE:600001"}`,
	} {
		f := newKernelFixture(t)
		watchlist := &apiWatchlist{}
		f.services.Watchlist = watchlist
		f.router = NewRouter(f.services)
		w := kernelRequest(t, f, "POST", "/api/v1/watchlist", body)
		require.Equal(t, 400, w.Code, body+": "+w.Body.String())
		require.Contains(t, w.Body.String(), "INVALID_REQUEST")
		require.Zero(t, watchlist.addCalls)
	}
}

func TestAddWatchlistItemMapsDomainErrors(t *testing.T) {
	for _, tt := range []struct {
		err     error
		status  int
		message string
	}{
		{port.ErrMarketDataNotFound, 404, "NOT_FOUND"},
		{application.ErrWatchlistFull, 409, "WATCHLIST_FULL"},
		{internalAPIError, 500, "internal server error"},
	} {
		f := newKernelFixture(t)
		f.services.Watchlist = &apiWatchlist{addErr: tt.err}
		f.router = NewRouter(f.services)
		w := kernelRequest(t, f, "POST", "/api/v1/watchlist", `{"instrument":"SSE:600000"}`)
		require.Equal(t, tt.status, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), tt.message)
		require.NotContains(t, w.Body.String(), "secret")
	}
}
