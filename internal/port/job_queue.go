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
}
