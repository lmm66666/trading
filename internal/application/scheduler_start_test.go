package application

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
)

func TestSchedulersWaitForFirstIntervalAndKeepTicking(t *testing.T) {
	for _, kind := range []string{"stocks", "futures"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := make(chan struct{}, 10)
				refresh := refreshFunc(func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
					calls <- struct{}{}
					return RefreshResult{Instrument: id, Version: 1}, nil
				})
				var start func(context.Context, time.Duration) error
				if kind == "stocks" {
					scheduler, _ := schedulerFixture(t, 1, refresh)
					start = func(ctx context.Context, interval time.Duration) error { return scheduler.Start(ctx, interval, 1) }
				} else {
					scheduler, err := NewFuturesScheduler(refresh, DefaultSinaFuturesInstruments()[:1], slog.New(slog.NewTextHandler(io.Discard, nil)))
					require.NoError(t, err)
					start = scheduler.Start
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- start(ctx, time.Hour) }()
				synctest.Wait()
				require.Empty(t, calls, "starting must not fetch market data")
				time.Sleep(time.Hour - time.Second)
				synctest.Wait()
				require.Empty(t, calls, "the first interval has not elapsed")
				time.Sleep(time.Second)
				synctest.Wait()
				require.Len(t, calls, 1, "first scheduled refresh must execute")
				time.Sleep(time.Hour)
				synctest.Wait()
				require.Len(t, calls, 2, "subsequent scheduled refresh must execute")
				cancel()
				synctest.Wait()
				require.ErrorIs(t, <-done, context.Canceled)
			})
		})
	}
}

func TestSchedulersCancelBeforeFirstIntervalWithoutRefresh(t *testing.T) {
	for _, kind := range []string{"stocks", "futures"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := make(chan struct{}, 10)
				refresh := refreshFunc(func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
					calls <- struct{}{}
					return RefreshResult{}, nil
				})
				var start func(context.Context, time.Duration) error
				if kind == "stocks" {
					s, _ := schedulerFixture(t, 1, refresh)
					start = func(ctx context.Context, d time.Duration) error { return s.Start(ctx, d, 1) }
				} else {
					s, err := NewFuturesScheduler(refresh, DefaultSinaFuturesInstruments()[:1], slog.New(slog.NewTextHandler(io.Discard, nil)))
					require.NoError(t, err)
					start = s.Start
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- start(ctx, time.Hour) }()
				synctest.Wait()
				time.Sleep(time.Minute)
				cancel()
				synctest.Wait()
				require.ErrorIs(t, <-done, context.Canceled)
				require.Empty(t, calls)
			})
		})
	}
}

func TestStockManualRefreshDuringInitialWaitDoesNotResetSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := make(chan struct{}, 10)
		s, _ := schedulerFixture(t, 1, func(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
			calls <- struct{}{}
			return RefreshResult{Instrument: id, Version: 1}, nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- s.Start(ctx, time.Hour, 1) }()
		synctest.Wait()
		require.Empty(t, calls)
		time.Sleep(30 * time.Minute)
		require.NoError(t, s.TriggerNow(ctx, 1))
		s.Wait()
		synctest.Wait()
		require.Len(t, calls, 1, "manual refresh must run before the first scheduled tick")
		time.Sleep(30 * time.Minute)
		synctest.Wait()
		require.Len(t, calls, 2, "manual refresh must not postpone the scheduled tick")
		cancel()
		synctest.Wait()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}
