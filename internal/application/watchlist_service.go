package application

import (
	"context"
	"errors"
	"fmt"

	"trading/internal/market"
	"trading/internal/port"
)

// ErrWatchlistFull reports that the watchlist already holds the maximum items.
var ErrWatchlistFull = errors.New("watchlist is full")

// WatchlistItem is a watchlisted instrument enriched with its latest daily quote.
type WatchlistItem struct {
	port.InstrumentSummary
	Close         *float64
	Change        *float64
	ChangePercent *float64
}

// WatchlistService orchestrates the single-user watchlist use cases.
type WatchlistService struct {
	store   port.WatchlistStore
	quotes  port.DailyQuoteReader
	catalog port.InstrumentCatalog
}

func NewWatchlistService(store port.WatchlistStore, quotes port.DailyQuoteReader, catalog port.InstrumentCatalog) (*WatchlistService, error) {
	if store == nil || quotes == nil || catalog == nil {
		return nil, invalidRequest("watchlist dependencies are required")
	}
	return &WatchlistService{store: store, quotes: quotes, catalog: catalog}, nil
}

func (s *WatchlistService) List(ctx context.Context) ([]WatchlistItem, error) {
	entries, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.InstrumentRowID)
	}
	quotes, err := s.quotes.LatestDailyQuotes(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]WatchlistItem, 0, len(entries))
	for _, entry := range entries {
		item := WatchlistItem{InstrumentSummary: entry.InstrumentSummary}
		if quote, ok := quotes[entry.InstrumentRowID]; ok {
			item.Close, item.Change, item.ChangePercent = quote.Close, quote.Change, quote.ChangePercent
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *WatchlistService) Add(ctx context.Context, id market.InstrumentID) ([]WatchlistItem, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidRequest("instrument is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	summary, err := s.catalog.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if summary.ID != id || !summary.Active {
		return nil, fmt.Errorf("%w: instrument %s", port.ErrMarketDataNotFound, id)
	}
	count, err := s.store.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count >= port.MaxWatchlistItems {
		return nil, ErrWatchlistFull
	}
	if _, err := s.store.Add(ctx, id); err != nil {
		return nil, err
	}
	return s.List(ctx)
}

func (s *WatchlistService) Remove(ctx context.Context, id market.InstrumentID) ([]WatchlistItem, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidRequest("instrument is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.store.Remove(ctx, id); err != nil {
		return nil, err
	}
	return s.List(ctx)
}
