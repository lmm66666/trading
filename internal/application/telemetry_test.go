package application

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"log/slog"
	"sync"
	"testing"
	"time"
	"trading/internal/port"
)

func TestTelemetryUsesOnlyDeclaredSafeFields(t *testing.T) {
	var output bytes.Buffer
	telemetry := SlogTelemetry{Logger: slog.New(slog.NewJSONHandler(&output, nil))}
	telemetry.ObserveStage(context.Background(), port.StageObservation{RunID: "run", Stage: "market_load", StrategyID: "test", StrategyVersion: "1", DataVersion: 7, Duration: time.Second, Rows: 3})
	require.Contains(t, output.String(), `"run_id":"run"`)
	require.Contains(t, output.String(), `"data_version":7`)
	require.NotContains(t, output.String(), "request_json")
	output.Reset()
	telemetry.CountRetry(context.Background(), "run", "evil password=secret", 1)
	require.NotContains(t, output.String(), "password")
	require.Contains(t, output.String(), "COMPUTE_FAILED")
}

type recordedTelemetry struct {
	mu      sync.Mutex
	stages  []port.StageObservation
	retries int
}

func (r *recordedTelemetry) ObserveStage(_ context.Context, v port.StageObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages = append(r.stages, v)
}
func (r *recordedTelemetry) CountRetry(context.Context, string, string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retries++
}
func TestComputeObservesStagesWithPinnedInputsAndDurations(t *testing.T) {
	s, _, st := newBacktestFixture(t)
	telemetry := &recordedTelemetry{}
	s.config.Telemetry = telemetry
	_, err := s.Create(context.Background(), computeRequest())
	require.NoError(t, err)
	run := st.claim()
	require.NoError(t, s.Execute(context.Background(), run))
	require.Len(t, telemetry.stages, 5)
	for _, o := range telemetry.stages {
		require.NoError(t, o.Validate())
		require.Equal(t, run.ID, o.RunID)
		require.Equal(t, run.DataVersion, o.DataVersion)
	}
	s.config.Clock = func() time.Time { return marketDate(1) }
	s.config.observe(context.Background(), run, "market_load", marketDate(2), 0, 0, 0, 0)
	require.Zero(t, telemetry.stages[len(telemetry.stages)-1].Duration)
}
func TestTelemetryRejectsUnknownStagesAndSupportsInstrumentAndRetry(t *testing.T) {
	var out bytes.Buffer
	telemetry := SlogTelemetry{Logger: slog.New(slog.NewJSONHandler(&out, nil))}
	o := port.StageObservation{RunID: "run", Stage: "unknown password=secret", StrategyID: "test", StrategyVersion: "1", DataVersion: 7}
	telemetry.ObserveStage(context.Background(), o)
	require.Empty(t, out.String())
	o.Stage = "feature_graph"
	o.Instrument = &marketID
	telemetry.ObserveStage(context.Background(), o)
	require.Contains(t, out.String(), `"instrument":"SSE:600000"`)
	telemetry.CountRetry(context.Background(), "run", "TEMPORARY", 1)
	require.Contains(t, out.String(), `"attempt":1`)
	SlogTelemetry{}.ObserveStage(context.Background(), o)
	SlogTelemetry{}.CountRetry(context.Background(), "run", "TEMPORARY", 1)
}
