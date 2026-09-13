package mysql

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"trading/internal/port"
)

var _ port.IdempotentRunReader = (*JobQueue)(nil)

func (q *JobQueue) FindByIdempotency(ctx context.Context, kind port.RunKind, key string) (port.Run, error) {
	if err := kind.Validate(); err != nil {
		return port.Run{}, err
	}
	if err := port.ValidateIdentity(key, "idempotency key", port.MaxIdempotencyKeyBytes, false); err != nil {
		return port.Run{}, err
	}
	var row ComputeRunModel
	err := q.db.WithContext(ctx).Where("kind = ? AND idempotency_key = ?", string(kind), key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = port.ErrRunNotFound
	}
	if err != nil {
		return port.Run{}, err
	}
	return row.run(), nil
}
