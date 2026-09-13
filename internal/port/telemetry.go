package port

import (
	"context"
	"strings"
	"time"

	"trading/internal/market"
)

type Telemetry interface {
	ObserveStage(ctx context.Context, observation StageObservation)
	CountRetry(ctx context.Context, runID, errorCode string, attempt int)
}

type StageObservation struct {
	RunID           string               `json:"run_id"`
	Stage           string               `json:"stage"`
	StrategyID      string               `json:"strategy_id"`
	StrategyVersion string               `json:"strategy_version"`
	Instrument      *market.InstrumentID `json:"instrument,omitempty"`
	DataVersion     market.DataVersion   `json:"data_version"`
	Duration        time.Duration        `json:"duration"`
	Rows            int64                `json:"rows"`
	Success         int64                `json:"success"`
	Failed          int64                `json:"failed"`
	Skipped         int64                `json:"skipped"`
}

func (observation StageObservation) Validate() error {
	if strings.TrimSpace(observation.RunID) == "" || strings.TrimSpace(observation.Stage) == "" ||
		strings.TrimSpace(observation.StrategyID) == "" || strings.TrimSpace(observation.StrategyVersion) == "" {
		return invalidPortValue("stage observation identity is required")
	}
	if observation.DataVersion == 0 || observation.Duration < 0 || observation.Rows < 0 || observation.Success < 0 || observation.Failed < 0 || observation.Skipped < 0 {
		return invalidPortValue("stage observation values are invalid")
	}
	if observation.Instrument != nil {
		return validateInstrument(*observation.Instrument, "stage instrument")
	}
	return nil
}
