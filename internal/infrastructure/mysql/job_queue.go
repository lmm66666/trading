package mysql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"trading/internal/market"
	"trading/internal/port"
)

type JobQueue struct{ db *gorm.DB }

func NewJobQueue(db *gorm.DB) *JobQueue { return &JobQueue{db: db} }

var _ port.JobQueue = (*JobQueue)(nil)

// retryTransaction retries only a rolled-back lock conflict. Commit failures
// are ambiguous and must never replay a possibly committed operation.
func retryTransaction(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		tx := db.WithContext(ctx).Begin()
		if tx.Error != nil {
			return tx.Error
		}
		err := fn(tx)
		if err == nil {
			return tx.Commit().Error
		}
		rollbackErr := tx.Rollback().Error
		var conflict *driver.MySQLError
		if rollbackErr != nil || attempt >= 2 || !errors.As(err, &conflict) || (conflict.Number != 1213 && conflict.Number != 1205) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (q *JobQueue) Enqueue(ctx context.Context, run port.Run) (port.Run, error) {
	if err := run.Validate(); err != nil {
		return port.Run{}, err
	}
	if run.Status != port.RunPending || run.CancelRequestedAt != nil || run.Attempts != 0 {
		return port.Run{}, invalid("enqueue requires an uncancelled pending run")
	}
	var stored ComputeRunModel
	err := retryTransaction(ctx, q.db, func(tx *gorm.DB) error {
		row := ComputeRunModel{RunID: run.ID, Kind: string(run.Kind), Status: string(port.RunPending), IdempotencyKey: run.IdempotencyKey, InputHash: run.InputHash, StrategyID: run.StrategyID, StrategyVersion: run.StrategyVersion, EngineVersion: run.EngineVersion, DataVersion: uint64(run.DataVersion), RequestJSON: append([]byte(nil), run.RequestJSON...)}
		if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"run_id": gorm.Expr("run_id")})}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Where("kind = ? AND idempotency_key = ?", string(run.Kind), run.IdempotencyKey).Take(&stored).Error; err != nil {
			return err
		}
		if stored.InputHash != run.InputHash {
			return port.ErrIdempotencyConflict
		}
		return nil
	})
	if err != nil {
		return port.Run{}, err
	}
	return stored.run(), nil
}

func leaseDuration(lease time.Duration) error {
	if lease < time.Microsecond {
		return invalid("lease must be at least one microsecond")
	}
	return nil
}
func runIdentity(runID, token string) error {
	if err := port.ValidateIdentity(runID, "run ID", port.MaxRunIDBytes, false); err != nil {
		return err
	}
	return port.ValidateIdentity(token, "lease token", port.MaxLeaseIdentityBytes, false)
}

func (q *JobQueue) Claim(ctx context.Context, owner string, lease time.Duration) (port.Run, error) {
	if err := port.ValidateIdentity(owner, "lease owner", port.MaxLeaseIdentityBytes, false); err != nil {
		return port.Run{}, err
	}
	if err := leaseDuration(lease); err != nil {
		return port.Run{}, err
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return port.Run{}, err
	}
	token := hex.EncodeToString(tokenBytes)
	var row ComputeRunModel
	err := retryTransaction(ctx, q.db, func(tx *gorm.DB) error {
		result := tx.Exec(fmt.Sprintf(`UPDATE t_compute_runs SET status='RUNNING', lease_owner=?, lease_token=?, lease_until=TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6)), attempts=attempts+1, updated_at=UTC_TIMESTAMP(6)
WHERE (status='PENDING' OR (status='RUNNING' AND lease_until <= UTC_TIMESTAMP(6))) AND cancel_requested_at IS NULL AND attempts < %[1]d AND (next_attempt_at IS NULL OR next_attempt_at <= UTC_TIMESTAMP(6)) ORDER BY created_at, id LIMIT 1`, port.MaxRunAttempts), owner, token, lease.Microseconds())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return port.ErrRunNotFound
		}
		return tx.Where("lease_owner = ? AND lease_token = ?", owner, token).Take(&row).Error
	})
	if err != nil {
		return port.Run{}, err
	}
	return row.run(), nil
}

func lockLease(tx *gorm.DB, runID, token string) (ComputeRunModel, error) {
	var row ComputeRunModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND lease_token = ? AND status = 'RUNNING' AND lease_owner <> '' AND lease_until > UTC_TIMESTAMP(6) AND cancel_requested_at IS NULL", runID, token).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = port.ErrLeaseLost
	}
	return row, err
}
func leaseCAS(tx *gorm.DB, row ComputeRunModel) *gorm.DB {
	return tx.Model(&ComputeRunModel{}).Where("run_id = ? AND lease_owner = ? AND lease_token = ? AND status = 'RUNNING' AND lease_until > UTC_TIMESTAMP(6) AND cancel_requested_at IS NULL", row.RunID, row.LeaseOwner, row.LeaseToken)
}
func affectedLease(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return port.ErrLeaseLost
	}
	return nil
}
func (q *JobQueue) Renew(ctx context.Context, runID, token string, lease time.Duration) error {
	if err := runIdentity(runID, token); err != nil {
		return err
	}
	if err := leaseDuration(lease); err != nil {
		return err
	}
	return retryTransaction(ctx, q.db, func(tx *gorm.DB) error {
		row, err := lockLease(tx, runID, token)
		if err != nil {
			return err
		}
		return affectedLease(leaseCAS(tx, row).Updates(map[string]any{"lease_until": gorm.Expr("TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6))", lease.Microseconds()), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}))
	})
}
func clearLease(status port.RunStatus) map[string]any {
	return map[string]any{"status": string(status), "lease_owner": "", "lease_token": "", "lease_until": nil, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}
}
func (q *JobQueue) Retry(ctx context.Context, runID, token string, next time.Time, failure port.Failure) error {
	if err := runIdentity(runID, token); err != nil {
		return err
	}
	if err := failure.Validate(); err != nil {
		return err
	}
	if next.IsZero() || next.Location() != time.UTC {
		return invalid("next attempt must be UTC")
	}
	return retryTransaction(ctx, q.db, func(tx *gorm.DB) error {
		row, err := lockLease(tx, runID, token)
		if err != nil {
			return err
		}
		status := port.RunPending
		if row.Attempts >= port.MaxRunAttempts || !failure.Retryable {
			status = port.RunFailed
		}
		update := clearLease(status)
		update["failure_code"] = failure.Code
		update["failure_message"] = failure.Message
		update["next_attempt_at"] = next
		return affectedLease(leaseCAS(tx, row).Updates(update))
	})
}

// RequestCancel terminalizes immediately while holding the run row lock. A
// computing worker can observe cancellation through Get and cannot publish.
func (q *JobQueue) RequestCancel(ctx context.Context, runID string) error {
	if err := port.ValidateIdentity(runID, "run ID", port.MaxRunIDBytes, false); err != nil {
		return err
	}
	return retryTransaction(ctx, q.db, func(tx *gorm.DB) error {
		var row ComputeRunModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return port.ErrRunNotFound
			}
			return err
		}
		if row.Status != string(port.RunPending) && row.Status != string(port.RunRunning) {
			return nil
		}
		update := clearLease(port.RunCancelled)
		update["cancel_requested_at"] = gorm.Expr("UTC_TIMESTAMP(6)")
		return tx.Model(&ComputeRunModel{}).Where("run_id = ? AND status = ? AND lease_owner = ? AND lease_token = ?", runID, row.Status, row.LeaseOwner, row.LeaseToken).Updates(update).Error
	})
}

// ReapExpired bounds each sweep to 1000 exhausted leases. Call periodically
// alongside workers; Claim itself never grants a fifth attempt.
func (q *JobQueue) ReapExpired(ctx context.Context) (int64, error) {
	var count int64
	err := retryTransaction(ctx, q.db, func(tx *gorm.DB) error {
		count = 0
		var rows []ComputeRunModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(fmt.Sprintf("status = 'RUNNING' AND attempts >= %d AND lease_until <= UTC_TIMESTAMP(6)", port.MaxRunAttempts)).Order("created_at, id").Limit(1000).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			status := port.RunFailed
			if row.CancelRequestedAt != nil {
				status = port.RunCancelled
			}
			update := clearLease(status)
			update["failure_code"] = "LEASE_EXHAUSTED"
			update["failure_message"] = "execution lease expired after maximum attempts"
			result := tx.Model(&ComputeRunModel{}).Where(fmt.Sprintf("run_id = ? AND lease_owner = ? AND lease_token = ? AND status = 'RUNNING' AND attempts >= %d AND lease_until <= UTC_TIMESTAMP(6)", port.MaxRunAttempts), row.RunID, row.LeaseOwner, row.LeaseToken).Updates(update)
			if err := affectedLease(result); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
func (row ComputeRunModel) run() port.Run {
	result := port.Run{ID: row.RunID, Kind: port.RunKind(row.Kind), Status: port.RunStatus(row.Status), IdempotencyKey: row.IdempotencyKey, InputHash: row.InputHash, StrategyID: row.StrategyID, StrategyVersion: row.StrategyVersion, EngineVersion: row.EngineVersion, DataVersion: market.DataVersion(row.DataVersion), RequestJSON: append([]byte(nil), row.RequestJSON...), LeaseOwner: row.LeaseOwner, LeaseToken: row.LeaseToken}
	if row.CancelRequestedAt != nil {
		at := row.CancelRequestedAt.UTC()
		result.CancelRequestedAt = &at
	}
	result.Attempts = int(row.Attempts)
	return result
}
