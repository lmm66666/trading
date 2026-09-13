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
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Kind) == "" || strings.TrimSpace(event.AggregateID) == "" {
		return invalidPortValue("event ID, kind and aggregate ID are required")
	}
	return validateUTCTime(event.OccurredAt, "event occurred at", false)
}
