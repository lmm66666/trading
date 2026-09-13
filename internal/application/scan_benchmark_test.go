package application

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trading/internal/port"
)

const checkedInLegacyFullScanBaseline = 40 * time.Second

func TestFullMarketScanPerformance(t *testing.T) {
	fixture := newFullMarketScanFixture(t, 5000, 140)
	started := time.Now()
	require.NoError(t, fixture.service.Execute(context.Background(), fixture.store.claim()))
	elapsed := time.Since(started)

	require.LessOrEqual(t, fixture.market.batchCalls, 1)
	require.Equal(t, 5000, fixture.market.batchIDs)
	require.Len(t, fixture.store.snapshot.Failures, 0)
	require.Less(t, elapsed, 8*time.Second)
	require.Less(t, elapsed*5, checkedInLegacyFullScanBaseline)
	observations := fixture.telemetry.observations()
	replays := 0
	for _, observation := range observations {
		if observation.Stage == "strategy_replay" {
			replays++
		}
	}
	require.Equal(t, 5000, replays)

	fixture.snapshots.snapshot = fixture.store.snapshot
	latencies := make([]time.Duration, 100)
	for i := range latencies {
		started = time.Now()
		_, err := fixture.snapshots.Latest(context.Background(), fixture.store.snapshot.Key, port.PageRequest{Limit: 100})
		require.NoError(t, err)
		latencies[i] = time.Since(started)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	require.Less(t, latencies[94], 200*time.Millisecond)
	t.Logf("full_scan=%s batch_reads=%d snapshot_p95=%s", elapsed, fixture.market.batchCalls, latencies[94])
}

func BenchmarkFullMarketScan5000(b *testing.B) {
	fixture := newFullMarketScanFixture(b, 5000, 140)
	fixture.service.config.Telemetry = nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := fixture.service.Execute(context.Background(), fixture.store.claim()); err != nil {
			b.Fatal(err)
		}
	}
}
