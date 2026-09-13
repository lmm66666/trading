package mysql

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"trading/internal/backtest"
	"trading/internal/port"
)

type RunStore struct{ db *gorm.DB }

func NewRunStore(db *gorm.DB) *RunStore { return &RunStore{db: db} }

var _ port.RunStore = (*RunStore)(nil)

func (s *RunStore) Get(ctx context.Context, runID string) (port.Run, error) {
	if err := port.ValidateIdentity(runID, "run ID", port.MaxRunIDBytes, false); err != nil {
		return port.Run{}, err
	}
	var row ComputeRunModel
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return port.Run{}, port.ErrRunNotFound
		}
		return port.Run{}, err
	}
	return row.run(), nil
}
func (s *RunStore) CompleteScan(ctx context.Context, runID, token string, snapshot port.SignalSnapshot) error {
	if err := runIdentity(runID, token); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if snapshot.RunID != runID {
		return invalid("snapshot belongs to a different run")
	}
	return retryTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, err := lockLease(tx, runID, token)
		if err != nil {
			return err
		}
		if row.Kind != string(port.RunScan) || row.StrategyID != snapshot.Key.StrategyID || row.StrategyVersion != snapshot.Key.StrategyVersion || row.DataVersion != uint64(snapshot.DataVersion) {
			return invalid("snapshot does not match run inputs")
		}
		now, err := databaseNow(tx)
		if err != nil {
			return err
		}
		status := port.RunSucceeded
		if len(snapshot.Failures) > 0 {
			status = port.RunPartialSucceeded
		}
		if err := insertSnapshot(tx, snapshot, status, now); err != nil {
			return err
		}
		if err := completionEvent(tx, row, status, snapshot.ID, now); err != nil {
			return err
		}
		return affectedLease(leaseCAS(tx, row).Updates(clearLease(status)))
	})
}
func (s *RunStore) CompleteBacktest(ctx context.Context, runID, token string, result backtest.Result) error {
	if err := runIdentity(runID, token); err != nil {
		return err
	}
	return retryTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, err := lockLease(tx, runID, token)
		if err != nil {
			return err
		}
		if row.Kind != string(port.RunBacktest) {
			return invalid("run is not a backtest")
		}
		if err := insertBacktest(tx, row, result); err != nil {
			return err
		}
		now, err := databaseNow(tx)
		if err != nil {
			return err
		}
		if err := completionEvent(tx, row, port.RunSucceeded, "", now); err != nil {
			return err
		}
		return affectedLease(leaseCAS(tx, row).Updates(clearLease(port.RunSucceeded)))
	})
}
func (s *RunStore) Fail(ctx context.Context, runID, token string, failure port.Failure) error {
	if err := runIdentity(runID, token); err != nil {
		return err
	}
	if err := failure.Validate(); err != nil {
		return err
	}
	return retryTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, err := lockLease(tx, runID, token)
		if err != nil {
			return err
		}
		now, err := databaseNow(tx)
		if err != nil {
			return err
		}
		if err := completionEvent(tx, row, port.RunFailed, "", now); err != nil {
			return err
		}
		update := clearLease(port.RunFailed)
		update["failure_code"] = failure.Code
		update["failure_message"] = failure.Message
		return affectedLease(leaseCAS(tx, row).Updates(update))
	})
}
