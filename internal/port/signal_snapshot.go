package port

import (
	"context"
	"math"
	"strings"
	"time"

	"trading/internal/market"
)

type SignalSnapshotStore interface {
	Latest(ctx context.Context, key SnapshotKey, page PageRequest) (SignalSnapshot, error)
}

// SnapshotKey selects an immutable published snapshot. A zero AsOf means the
// newest published snapshot matching StrategyID, StrategyVersion and ParametersHash.
type SnapshotKey struct {
	StrategyID      string    `json:"strategy_id"`
	StrategyVersion string    `json:"strategy_version"`
	ParametersHash  string    `json:"parameters_hash"`
	AsOf            time.Time `json:"as_of,omitempty"`
}

func (key SnapshotKey) Validate() error {
	if strings.TrimSpace(key.StrategyID) == "" || strings.TrimSpace(key.StrategyVersion) == "" || strings.TrimSpace(key.ParametersHash) == "" {
		return invalidPortValue("snapshot strategy identity and parameters hash are required")
	}
	return validateUTCTime(key.AsOf, "snapshot as-of", true)
}

type SnapshotRow struct {
	Instrument market.InstrumentID `json:"instrument"`
	SignalTime time.Time           `json:"signal_time"`
	Reason     string              `json:"reason"`
	Values     map[string]float64  `json:"values,omitempty"`
}

func (row SnapshotRow) Validate() error {
	if err := validateInstrument(row.Instrument, "snapshot instrument"); err != nil {
		return err
	}
	if err := validateUTCTime(row.SignalTime, "signal time", false); err != nil {
		return err
	}
	if strings.TrimSpace(row.Reason) == "" {
		return invalidPortValue("snapshot reason is required")
	}
	for name, value := range row.Values {
		if strings.TrimSpace(name) == "" || math.IsNaN(value) || math.IsInf(value, 0) {
			return invalidPortValue("snapshot value is invalid")
		}
	}
	return nil
}

// SignalSnapshot is an immutable published scan output. Rows and Failures are
// response-owned; stores return fresh maps/slices and callers do not mutate a
// value retained by an adapter.
type SignalSnapshot struct {
	ID          string                          `json:"id"`
	RunID       string                          `json:"run_id"`
	Key         SnapshotKey                     `json:"key"`
	DataVersion market.DataVersion              `json:"data_version"`
	Rows        []SnapshotRow                   `json:"rows"`
	Failures    map[market.InstrumentID]Failure `json:"failures,omitempty"`
}

func (snapshot SignalSnapshot) Validate() error {
	if strings.TrimSpace(snapshot.ID) == "" || strings.TrimSpace(snapshot.RunID) == "" {
		return invalidPortValue("snapshot ID and run ID are required")
	}
	if err := snapshot.Key.Validate(); err != nil {
		return err
	}
	if snapshot.DataVersion == 0 {
		return invalidPortValue("snapshot data version is required")
	}
	rows := make(map[market.InstrumentID]struct{}, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		if err := row.Validate(); err != nil {
			return err
		}
		if _, duplicate := rows[row.Instrument]; duplicate {
			return invalidPortValue("duplicate snapshot instrument %s", row.Instrument)
		}
		rows[row.Instrument] = struct{}{}
	}
	for id, failure := range snapshot.Failures {
		if err := validateInstrument(id, "failed instrument"); err != nil {
			return err
		}
		if _, success := rows[id]; success {
			return invalidPortValue("instrument cannot be both successful and failed")
		}
		if err := failure.Validate(); err != nil {
			return err
		}
	}
	return nil
}
