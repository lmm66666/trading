package application

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
	"trading/internal/strategy/builtin"
)

type fullMarketScanFixture struct {
	service   *ScanService
	market    *fullMarketData
	store     *computeStore
	snapshots *computeSnapshots
	telemetry *performanceTelemetry
}

type fullMarketData struct {
	port.MarketData
	ids        []market.InstrumentID
	barCount   int
	version    market.DataVersion
	from       time.Time
	batchCalls int
	batchIDs   int
}

func (m *fullMarketData) LatestCompleteVersion(context.Context) (market.DataVersion, error) {
	return m.version, nil
}

func (m *fullMarketData) Instruments(context.Context, port.InstrumentScope) ([]market.InstrumentID, error) {
	return append([]market.InstrumentID(nil), m.ids...), nil
}

func (m *fullMarketData) BatchDatasets(ctx context.Context, ids []market.InstrumentID, request port.BatchRequest) (map[market.InstrumentID]port.Bundle, map[market.InstrumentID]error) {
	m.batchCalls++
	m.batchIDs += len(ids)
	bundles := make(map[market.InstrumentID]port.Bundle, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return bundles, map[market.InstrumentID]error{id: err}
		}
		bars := make([]market.Bar, m.barCount)
		for index := range bars {
			at := m.from.AddDate(0, 0, index)
			price := market.Price(100_000 + index*10)
			bars[index] = market.Bar{
				Instrument: id,
				Timeframe:  request.PrimaryTimeframe,
				OpenTime:   at,
				CloseTime:  at,
				Open:       price,
				High:       price + 1_000,
				Low:        price - 1_000,
				Close:      price,
				Volume:     int64(1_000 + index),
				Amount:     market.Money(price) * 1_000,
				Version:    request.Version,
			}
		}
		dataset, err := market.NewDataset(id, request.PrimaryTimeframe, request.Version, bars)
		if err != nil {
			return bundles, map[market.InstrumentID]error{id: err}
		}
		bundles[id] = port.Bundle{
			Primary: dataset,
			Factors: []market.AdjustmentFactor{{EffectiveTime: m.from, Numerator: 1, Denominator: 1, Version: request.Version}},
			Quality: port.DataComplete,
		}
	}
	return bundles, map[market.InstrumentID]error{}
}

type performanceTelemetry struct {
	mu     sync.Mutex
	stages []port.StageObservation
}

func (t *performanceTelemetry) ObserveStage(_ context.Context, observation port.StageObservation) {
	t.mu.Lock()
	t.stages = append(t.stages, observation)
	t.mu.Unlock()
}

func (*performanceTelemetry) CountRetry(context.Context, string, string, int) {}

func (t *performanceTelemetry) observations() []port.StageObservation {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]port.StageObservation(nil), t.stages...)
}

func newFullMarketScanFixture(tb testing.TB, instrumentCount, barCount int) *fullMarketScanFixture {
	tb.Helper()
	ids := make([]market.InstrumentID, instrumentCount)
	for index := range ids {
		ids[index] = market.InstrumentID{Exchange: market.SSE, Code: fmt.Sprintf("%06d", 600_000+index)}
	}
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, barCount-1)
	marketData := &fullMarketData{ids: ids, barCount: barCount, version: 7, from: from}
	store := &computeStore{}
	snapshots := &computeSnapshots{}
	telemetry := &performanceTelemetry{}
	registry := &strategy.Registry{}
	require.NoError(tb, builtin.RegisterAll(registry))
	service, err := NewScanService(registry, marketData, store, store, snapshots, ScanConfig{
		ComputeConfig: ComputeConfig{EngineVersion: "performance-v1", Telemetry: telemetry},
		Workers:       8,
	})
	require.NoError(tb, err)
	_, err = service.Create(context.Background(), ScanRequest{
		StrategyID:      "daily_b1_buy",
		StrategyVersion: "1",
		IdempotencyKey:  "full-market-performance",
		From:            from,
		AsOf:            to,
		Scope:           port.InstrumentScope{Exchanges: []market.Exchange{market.SSE}, ActiveOnly: true, Limit: instrumentCount},
	})
	require.NoError(tb, err)
	return &fullMarketScanFixture{service: service, market: marketData, store: store, snapshots: snapshots, telemetry: telemetry}
}
