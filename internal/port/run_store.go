package port

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"trading/internal/backtest"
	"trading/internal/market"
)

type RunStore interface {
	CompleteBacktest(ctx context.Context, runID, leaseToken string, result backtest.Result) error
	CompleteScan(ctx context.Context, runID, leaseToken string, snapshot SignalSnapshot) error
	Fail(ctx context.Context, runID, leaseToken string, failure Failure) error
	Get(ctx context.Context, runID string) (Run, error)
	BacktestResult(ctx context.Context, runID string) (backtest.Summary, error)
	Orders(ctx context.Context, runID string, page PageRequest) (Page[backtest.Order], error)
	Trades(ctx context.Context, runID string, page PageRequest) (Page[backtest.Fill], error)
	Equity(ctx context.Context, runID string, page PageRequest) (Page[backtest.EquityPoint], error)
}

type RunKind string

const (
	RunBacktest RunKind = "BACKTEST"
	RunScan     RunKind = "SCAN"
)

func (kind RunKind) Validate() error {
	if kind != RunBacktest && kind != RunScan {
		return invalidPortValue("unknown run kind %q", kind)
	}
	return nil
}

type RunStatus string

const (
	RunPending          RunStatus = "PENDING"
	RunRunning          RunStatus = "RUNNING"
	RunSucceeded        RunStatus = "SUCCEEDED"
	RunPartialSucceeded RunStatus = "PARTIAL_SUCCEEDED"
	RunFailed           RunStatus = "FAILED"
	RunCancelled        RunStatus = "CANCELLED"
)

func (status RunStatus) Validate() error {
	switch status {
	case RunPending, RunRunning, RunSucceeded, RunPartialSucceeded, RunFailed, RunCancelled:
		return nil
	default:
		return invalidPortValue("unknown run status %q", status)
	}
}

// Run holds all inputs required to reproduce an asynchronous execution. Its
// RequestJSON is a caller-owned serialized DTO; queues and stores copy it.
// Attempts is assigned by durable queues; callers enqueue with zero.
type Run struct {
	ID                string             `json:"id"`
	IdempotencyKey    string             `json:"idempotency_key"`
	InputHash         string             `json:"input_hash"`
	Kind              RunKind            `json:"kind"`
	Status            RunStatus          `json:"status"`
	StrategyID        string             `json:"strategy_id"`
	StrategyVersion   string             `json:"strategy_version"`
	EngineVersion     string             `json:"engine_version"`
	DataVersion       market.DataVersion `json:"data_version"`
	RequestJSON       []byte             `json:"request_json"`
	Attempts          int                `json:"attempts"`
	LeaseOwner        string             `json:"lease_owner,omitempty"`
	LeaseToken        string             `json:"lease_token,omitempty"`
	CancelRequestedAt *time.Time         `json:"cancel_requested_at,omitempty"`
}

func (run Run) Validate() error {
	if run.Attempts < 0 || run.Attempts > 4 {
		return invalidPortValue("run attempts must be between zero and four")
	}
	for _, field := range []struct {
		name, value string
		limit       int
		optional    bool
	}{
		{"run ID", run.ID, MaxRunIDBytes, false}, {"idempotency key", run.IdempotencyKey, MaxIdempotencyKeyBytes, false}, {"input hash", run.InputHash, MaxHashBytes, false},
		{"strategy ID", run.StrategyID, MaxStrategyIDBytes, false}, {"strategy version", run.StrategyVersion, MaxStrategyVersionBytes, false}, {"engine version", run.EngineVersion, MaxEngineVersionBytes, false},
		{"lease owner", run.LeaseOwner, MaxLeaseIdentityBytes, true}, {"lease token", run.LeaseToken, MaxLeaseIdentityBytes, true},
	} {
		if err := ValidateIdentity(field.value, field.name, field.limit, field.optional); err != nil {
			return err
		}
	}
	if err := run.Kind.Validate(); err != nil {
		return err
	}
	if err := run.Status.Validate(); err != nil {
		return err
	}
	if run.DataVersion == 0 {
		return invalidPortValue("data version is required")
	}
	if !json.Valid(run.RequestJSON) {
		return invalidPortValue("request JSON must be valid JSON")
	}
	if run.CancelRequestedAt != nil {
		if err := validateUTCTime(*run.CancelRequestedAt, "cancel requested at", false); err != nil {
			return err
		}
	}
	leaseOwner := strings.TrimSpace(run.LeaseOwner)
	leaseToken := strings.TrimSpace(run.LeaseToken)
	if (run.LeaseOwner != "" && leaseOwner == "") || (run.LeaseToken != "" && leaseToken == "") {
		return invalidPortValue("lease owner and token cannot be whitespace")
	}
	switch run.Status {
	case RunRunning:
		if leaseOwner == "" || leaseToken == "" {
			return invalidPortValue("running run requires lease owner and token")
		}
	default:
		if leaseOwner != "" || leaseToken != "" {
			return invalidPortValue("non-running run cannot have a lease")
		}
	}
	return nil
}

type Failure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (failure Failure) Validate() error {
	if strings.TrimSpace(failure.Code) == "" || strings.TrimSpace(failure.Message) == "" {
		return invalidPortValue("failure code and message are required")
	}
	return nil
}

type PageRequest struct {
	AfterSequence int64 `json:"after_sequence,omitempty"`
	Limit         int   `json:"limit"`
}

func (request PageRequest) Validate() error {
	if request.AfterSequence < 0 || request.Limit < 1 || request.Limit > MaxPageSize {
		return invalidPortValue("page cursor must be non-negative and limit must be between 1 and %d", MaxPageSize)
	}
	return nil
}

type Page[T any] struct {
	Items        []T    `json:"items"`
	NextSequence *int64 `json:"next_sequence,omitempty"`
}
