package application

import (
	"context"
	"log/slog"
	"time"
	"trading/internal/port"
)

// SlogTelemetry deliberately has no request, config, error-message or arbitrary
// attribute input. Failure classifications and stages are allowlisted.
type SlogTelemetry struct{ Logger *slog.Logger }

func (t SlogTelemetry) ObserveStage(ctx context.Context, o port.StageObservation) {
	if t.Logger == nil || o.Validate() != nil {
		return
	}
	switch o.Stage {
	case "queue_wait", "market_load", "dataset_validation", "feature_graph", "strategy_replay", "engine_execution", "persistence":
	default:
		return
	}
	attrs := []any{"run_id", o.RunID, "stage", o.Stage, "strategy_id", o.StrategyID, "strategy_version", o.StrategyVersion, "data_version", o.DataVersion, "duration_ns", o.Duration.Nanoseconds(), "rows", o.Rows, "success", o.Success, "failed", o.Failed, "skipped", o.Skipped}
	if o.Instrument != nil {
		attrs = append(attrs, "instrument", o.Instrument.String())
	}
	t.Logger.InfoContext(ctx, "compute stage", attrs...)
}
func (t SlogTelemetry) CountRetry(ctx context.Context, id, code string, attempt int) {
	if t.Logger == nil {
		return
	}
	switch code {
	case "TEMPORARY", "TIMEOUT", "CONNECTION":
	default:
		code = "COMPUTE_FAILED"
	}
	t.Logger.InfoContext(ctx, "compute retry", "run_id", id, "error_code", code, "attempt", attempt)
}
func (c ComputeConfig) observe(ctx context.Context, run port.Run, stage string, start time.Time, rows, success, failed, skipped int64) {
	if nilComputeDependency(c.Telemetry) {
		return
	}
	duration := c.Clock().Sub(start)
	if duration < 0 {
		duration = 0
	}
	c.Telemetry.ObserveStage(ctx, port.StageObservation{RunID: run.ID, Stage: stage, StrategyID: run.StrategyID, StrategyVersion: run.StrategyVersion, DataVersion: run.DataVersion, Duration: duration, Rows: rows, Success: success, Failed: failed, Skipped: skipped})
}
