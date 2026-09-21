package application

import (
	"context"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func TestUnifiedRefreshStartsIdleKindsAndDoesNotDuplicateRunningKinds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan market.InstrumentID, 10)
	refresh := refreshFunc(func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
		entered <- id
		<-ctx.Done()
		return RefreshResult{}, ctx.Err()
	})
	stock, _ := schedulerFixture(t, 1, refresh)
	futures, err := NewFuturesScheduler(refresh, DefaultSinaFuturesInstruments()[:1], slog.Default())
	require.NoError(t, err)
	defer func() { cancel(); stock.Wait(); futures.Wait() }()
	first, err := TriggerMarketRefresh(ctx, stock, nil, 1)
	require.NoError(t, err)
	require.Equal(t, "ACCEPTED", first.Stock.Status)
	require.Equal(t, "DISABLED", first.Futures.Status)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("stock did not start")
	}
	second, err := TriggerMarketRefresh(ctx, stock, futures, 1)
	require.NoError(t, err)
	require.Equal(t, "ALREADY_RUNNING", second.Stock.Status)
	require.Equal(t, "ACCEPTED", second.Futures.Status)
	require.NotEmpty(t, second.Futures.RunID)
	select {
	case id := <-entered:
		require.Equal(t, market.FuturesContinuous, id.Kind())
	case <-time.After(time.Second):
		t.Fatal("futures did not start")
	}
	third, err := TriggerMarketRefresh(ctx, stock, futures, 1)
	require.NoError(t, err)
	require.Equal(t, "ALREADY_RUNNING", third.Stock.Status)
	require.Equal(t, "ALREADY_RUNNING", third.Futures.Status)
	require.ErrorIs(t, futures.RunOnce(ctx).Err, ErrRefreshAlreadyRunning)
	cancel()
	stock.Wait()
	futures.Wait()
	require.ErrorIs(t, futures.LastSummary().Err, context.Canceled)
	_, err = TriggerMarketRefresh(ctx, stock, futures, 1)
	require.ErrorIs(t, err, context.Canceled)
}

func TestUnifiedRefreshValidatesBeforeStartingEitherKind(t *testing.T) {
	stock, _ := schedulerFixture(t, 1, func(context.Context, market.InstrumentID) (RefreshResult, error) {
		t.Error("must not start")
		return RefreshResult{}, nil
	})
	_, err := TriggerMarketRefresh(context.Background(), stock, nil, 0)
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, err = TriggerMarketRefresh(context.Background(), nil, nil, 1)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestUnifiedRefreshStartsBothKindsAndRecordsManualSource(t *testing.T) {
	ctx := context.Background()
	refresh := refreshFunc(func(context.Context, market.InstrumentID) (RefreshResult, error) { return RefreshResult{}, nil })
	stock, _ := schedulerFixture(t, 1, refresh)
	futures, err := NewFuturesScheduler(refresh, DefaultSinaFuturesInstruments(), slog.Default())
	require.NoError(t, err)
	store := &progressMemory{}
	progress := NewRefreshProgress(store)
	stock.SetProgress(progress)
	futures.SetProgress(progress)
	receipt, err := TriggerMarketRefresh(ctx, stock, futures, 1)
	require.NoError(t, err)
	require.Equal(t, "ACCEPTED", receipt.Stock.Status)
	require.Equal(t, "ACCEPTED", receipt.Futures.Status)
	require.NotEqual(t, receipt.Stock.RunID, receipt.Futures.RunID)
	stock.Wait()
	futures.Wait()
	progress.Flush(ctx)
	require.Len(t, stock.LastSummary().Results, 1)
	require.Len(t, futures.LastSummary().Results, 8)
	require.NotEmpty(t, store.snapshots)
	for _, snapshot := range store.snapshots {
		require.Equal(t, "MANUAL", snapshot.Run.Trigger)
	}
}

func TestUnifiedRefreshFailureDoesNotHideAcceptedStock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stock, _ := schedulerFixture(t, 1, func(context.Context, market.InstrumentID) (RefreshResult, error) { return RefreshResult{}, nil })
	// Cancel between the two triggers through the initial stock observation save.
	stock.SetProgress(NewRefreshProgress(cancelProgressWriter{cancel}))
	futures, err := NewFuturesScheduler(&futuresRefresherFake{}, DefaultSinaFuturesInstruments(), slog.Default())
	require.NoError(t, err)
	receipt, err := TriggerMarketRefresh(ctx, stock, futures, 1)
	require.NoError(t, err)
	stock.Wait()
	require.Equal(t, "ACCEPTED", receipt.Stock.Status)
	require.Equal(t, "FAILED", receipt.Futures.Status)
	require.Equal(t, "REFRESH_UNAVAILABLE", receipt.Futures.ErrorCode)
}

type cancelProgressWriter struct{ cancel context.CancelFunc }

func (w cancelProgressWriter) SaveRefresh(context.Context, port.RefreshSnapshot) error {
	w.cancel()
	return nil
}
