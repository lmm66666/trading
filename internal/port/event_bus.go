package port

import (
	"context"
	"strings"
	"time"
)

type EventBus interface {
	Publish(ctx context.Context, event Event) error
}

// Event is an opaque, serializable integration message. Payload bytes remain
// owned by the caller; a bus copies them before asynchronous publication.
type Event struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	AggregateID string    `json:"aggregate_id"`
	Payload     []byte    `json:"payload"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func (event Event) Validate() error {
	if err := ValidateIdentity(event.ID, "event ID", MaxEventIDBytes, false); err != nil {
		return err
	}
	if err := ValidateIdentity(event.AggregateID, "aggregate ID", MaxAggregateIDBytes, false); err != nil {
		return err
	}
	if strings.TrimSpace(event.Kind) == "" {
		return invalidPortValue("event kind is required")
	}
	return validateUTCTime(event.OccurredAt, "event occurred at", false)
}
