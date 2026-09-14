package application

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"trading/internal/market"
)

type futuresRefresherFake struct {
	mu       sync.Mutex
	calls    []market.InstrumentID
	failures map[market.InstrumentID]error
	started  chan struct{}
	release  chan struct{}
}

func (fake *futuresRefresherFake) Refresh(ctx context.Context, id market.InstrumentID) (RefreshResult, error) {
	fake.mu.Lock()
	fake.calls = append(fake.calls, id)
	fake.mu.Unlock()
	if fake.started != nil {
		select {
		case fake.started <- struct{}{}:
		default:
		}
	}
	if fake.release != nil {
		select {
		case <-fake.release:
		case <-ctx.Done():
			return RefreshResult{}, ctx.Err()
		}
	}
	if err := fake.failures[id]; err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{Instrument: id, Version: 1}, nil
}

func TestFuturesSchedulerRefreshesConfiguredMainSeriesInOrder(t *testing.T) {
	ids := DefaultSinaFuturesInstruments()
	failure := errors.New("stale upstream")
	fake := &futuresRefresherFake{failures: map[market.InstrumentID]error{ids[2]: failure}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	scheduler, err := NewFuturesScheduler(fake, ids, logger)
	require.NoError(t, err)
	summary := scheduler.RunOnce(context.Background())
	require.NoError(t, summary.Err)
	require.Equal(t, len(ids), summary.Total)
	require.Equal(t, ids, fake.calls)
	require.ErrorIs(t, summary.Failures[ids[2]], failure)
	require.Len(t, summary.Results, len(ids)-1)
	require.Contains(t, logs.String(), `"component":"futures_scheduler"`)
	require.Contains(t, logs.String(), `"operation":"refresh"`)
	require.Contains(t, logs.String(), `"total":8`)
	require.Contains(t, logs.String(), `"succeeded":7`)
	require.Contains(t, logs.String(), `"failed":1`)
	require.Contains(t, logs.String(), `"duration_ms":`)
}

func TestFuturesSchedulerRejectsInvalidOrDuplicateIdentity(t *testing.T) {
	fake := &futuresRefresherFake{}
	_, err := NewFuturesScheduler(fake, []market.InstrumentID{{Exchange: market.SHFE, Code: "AU202612"}}, slog.Default())
	require.Error(t, err)
	id := market.InstrumentID{Exchange: market.SHFE, Code: "AU.MAIN"}
	_, err = NewFuturesScheduler(fake, []market.InstrumentID{id, id}, slog.Default())
	require.Error(t, err)
	_, err = NewFuturesScheduler(fake, []market.InstrumentID{id}, nil)
	require.Error(t, err)
}

func TestFuturesSchedulerPreventsOverlappingRunsAndCancels(t *testing.T) {
	fake := &futuresRefresherFake{started: make(chan struct{}, 1), release: make(chan struct{})}
	scheduler, err := NewFuturesScheduler(fake, DefaultSinaFuturesInstruments()[:1], slog.Default())
	require.NoError(t, err)
	done := make(chan RefreshSummary, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { done <- scheduler.RunOnce(ctx) }()
	<-fake.started
	require.ErrorIs(t, scheduler.RunOnce(context.Background()).Err, ErrRefreshAlreadyRunning)
	cancel()
	summary := <-done
	require.ErrorIs(t, summary.Err, context.Canceled)
}

func TestFuturesSchedulerStartRunsImmediatelyAndStops(t *testing.T) {
	fake := &futuresRefresherFake{started: make(chan struct{}, 1)}
	scheduler, err := NewFuturesScheduler(fake, DefaultSinaFuturesInstruments()[:1], slog.Default())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- scheduler.Start(ctx, time.Hour) }()
	<-fake.started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
