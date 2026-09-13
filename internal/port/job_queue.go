package port

import (
	"context"
	"time"
)

type JobQueue interface {
	Enqueue(ctx context.Context, run Run) (Run, error)
	Claim(ctx context.Context, owner string, lease time.Duration) (Run, error)
	Renew(ctx context.Context, runID, leaseToken string, lease time.Duration) error
	Retry(ctx context.Context, runID, leaseToken string, nextAttempt time.Time, failure Failure) error
	RequestCancel(ctx context.Context, runID string) error
	// ReapExpired terminalizes a bounded batch of exhausted expired leases.
	ReapExpired(ctx context.Context) (int64, error)
}

// IdempotentRunReader resolves a previous submission before the application
// consults a newer market version or a changed instrument universe. Enqueue's
// atomic uniqueness check still arbitrates concurrent first submissions.
type IdempotentRunReader interface {
	FindByIdempotency(ctx context.Context, kind RunKind, key string) (Run, error)
}
