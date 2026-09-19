package api

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
)

func TestRemoveWatchlistItemDecodesPathIdentity(t *testing.T) {
	f := newKernelFixture(t)
	watchlist := &apiWatchlist{items: watchlistFixtureItems()}
	f.services.Watchlist = watchlist
	f.router = NewRouter(f.services)

	path := "/api/v1/watchlist/" + url.PathEscape("SSE:600000")
	w := kernelRequest(t, f, "DELETE", path, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 1, watchlist.removeCalls)
	require.Equal(t, market.InstrumentID{Exchange: market.SSE, Code: "600000"}, watchlist.removedID)
	require.Contains(t, w.Body.String(), `"instrument":"SSE:600000"`)
}

func TestRemoveWatchlistItemRejectsInvalidIdentityAndMapsErrors(t *testing.T) {
	f := newKernelFixture(t)
	watchlist := &apiWatchlist{}
	f.services.Watchlist = watchlist
	f.router = NewRouter(f.services)
	w := kernelRequest(t, f, "DELETE", "/api/v1/watchlist/600000", "")
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "INVALID_REQUEST")
	require.Zero(t, watchlist.removeCalls)

	watchlist.removeErr = application.ErrWatchlistFull
	w = kernelRequest(t, f, "DELETE", "/api/v1/watchlist/SSE%3A600000", "")
	require.Equal(t, 409, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "WATCHLIST_FULL")

	f.services.Watchlist = nil
	f.router = NewRouter(f.services)
	w = kernelRequest(t, f, "DELETE", "/api/v1/watchlist/SSE%3A600000", "")
	require.Equal(t, 500, w.Code)
}
