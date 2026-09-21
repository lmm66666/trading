package mysql

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
	"trading/internal/port"
)

type RefreshProgressStore struct{ db *gorm.DB }

func NewRefreshProgressStore(db *gorm.DB) *RefreshProgressStore { return &RefreshProgressStore{db: db} }
func (s *RefreshProgressStore) SaveRefresh(ctx context.Context, snapshot port.RefreshSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := refreshRunModel(snapshot.Run)
		row.ID = 0
		row.Revision = 0
		row.State = "PREPARING"
		row.FinishedAt = nil
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var current RefreshRunModel
		if err := tx.Select(refreshRunColumns).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", snapshot.Run.RunID).Take(&current).Error; err != nil {
			return err
		}
		if current.Revision >= snapshot.Run.Revision || !current.dto().Active() {
			return nil
		}
		if current.Kind != snapshot.Run.Kind || current.Trigger != snapshot.Run.Trigger || current.Succeeded > snapshot.Run.Succeeded || current.Failed > snapshot.Run.Failed {
			return port.ErrInvalidPortValue
		}
		failures := make([]RefreshFailureModel, 0, len(snapshot.Failures))
		for _, f := range snapshot.Failures {
			failures = append(failures, RefreshFailureModel{RunID: f.RunID, Exchange: string(f.Exchange), Code: f.Code, ErrorCode: f.ErrorCode, CompletedAt: f.CompletedAt})
		}
		if len(failures) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(failures, 100).Error; err != nil {
				return err
			}
		}
		next := refreshRunModel(snapshot.Run)
		return tx.Model(&RefreshRunModel{}).Where("id = ?", current.ID).Select(strings.Split(strings.TrimPrefix(refreshRunColumns, "id,"), ",")).Updates(&next).Error
	})
}
func (s *RefreshProgressStore) InterruptRefreshRunsBefore(ctx context.Context, before time.Time) error {
	// The single-updater deployment contract makes all pre-start active rows stale.
	now := time.Now().UTC().Truncate(time.Microsecond)
	return s.db.WithContext(ctx).Model(&RefreshRunModel{}).Where("state IN ? AND started_at < ?", []string{"PREPARING", "RUNNING"}, before).Updates(map[string]any{"state": "INTERRUPTED", "finished_at": now, "error_code": "INTERRUPTED", "snapshot_at": now, "revision": gorm.Expr("revision + 1")}).Error
}
func (s *RefreshProgressStore) ListRefreshRuns(ctx context.Context, kind string, before uint64, limit int) ([]port.RefreshRun, error) {
	if (kind != "" && kind != "STOCK" && kind != "FUTURES") || limit < 1 || limit > 100 {
		return nil, port.ErrInvalidPortValue
	}
	q := s.db.WithContext(ctx).Select(refreshRunColumns)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if before > 0 {
		q = q.Where("id < ?", before)
	}
	var rows []RefreshRunModel
	if err := q.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]port.RefreshRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.dto())
	}
	return out, nil
}
func (s *RefreshProgressStore) GetRefreshRun(ctx context.Context, id string) (port.RefreshRun, error) {
	if port.ValidateIdentity(id, "run_id", 64, false) != nil {
		return port.RefreshRun{}, port.ErrInvalidPortValue
	}
	var row RefreshRunModel
	err := s.db.WithContext(ctx).Select(refreshRunColumns).Where("run_id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return port.RefreshRun{}, port.ErrRefreshRunNotFound
	}
	return row.dto(), err
}
func (s *RefreshProgressStore) ListRefreshFailures(ctx context.Context, id string, after uint64, limit int) ([]port.RefreshFailure, error) {
	if limit < 1 || limit > 100 {
		return nil, port.ErrInvalidPortValue
	}
	if _, err := s.GetRefreshRun(ctx, id); err != nil {
		return nil, err
	}
	rows := []port.RefreshFailure{}
	err := s.db.WithContext(ctx).Table("t_market_refresh_failures AS f").Select("f.id, f.run_id, f.exchange, f.code, f.error_code, f.completed_at, COALESCE(i.name, '') AS name").Joins("LEFT JOIN t_instruments AS i ON i.exchange = f.exchange AND i.code = f.code").Where("f.run_id = ? AND f.id > ?", id, after).Order("f.id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

func (s *RefreshProgressStore) LatestRefreshRun(ctx context.Context, kind string) (port.RefreshRun, error) {
	if kind != "STOCK" && kind != "FUTURES" {
		return port.RefreshRun{}, port.ErrInvalidPortValue
	}
	var row RefreshRunModel
	err := s.db.WithContext(ctx).Select(refreshRunColumns).Where("kind = ?", kind).Order("started_at DESC, id DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return port.RefreshRun{}, port.ErrRefreshRunNotFound
	}
	return row.dto(), err
}
