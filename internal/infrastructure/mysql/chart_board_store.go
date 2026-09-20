package mysql

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"trading/internal/port"
)

// ChartBoardStore persists the single-user chart boards and keeps the
// is_active exactly-one invariant inside one transaction per mutation.
type ChartBoardStore struct{ db *gorm.DB }

var _ port.ChartBoardStore = (*ChartBoardStore)(nil)

func NewChartBoardStore(db *gorm.DB) *ChartBoardStore { return &ChartBoardStore{db: db} }

func chartBoardErrNotFound(id uint64) error {
	return fmt.Errorf("%w: chart board %d", port.ErrChartBoardNotFound, id)
}

func (s *ChartBoardStore) List(ctx context.Context) (port.ChartBoardState, error) {
	var rows []ChartBoardModel
	if err := s.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return port.ChartBoardState{}, fmt.Errorf("list chart boards: %w", err)
	}
	state := port.ChartBoardState{Boards: make([]port.ChartBoard, 0, len(rows))}
	for _, row := range rows {
		state.Boards = append(state.Boards, port.ChartBoard{ID: row.ID, Name: row.Name, Config: row.Config})
		if row.IsActive {
			state.ActiveID = row.ID
		}
	}
	return state, nil
}

func (s *ChartBoardStore) Create(ctx context.Context, name, config string) (port.ChartBoardState, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := ChartBoardModel{Name: name, Config: config, IsActive: true}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("create chart board: %w", err)
		}
		if err := tx.Model(&ChartBoardModel{}).Where("id <> ?", row.ID).UpdateColumn("is_active", false).Error; err != nil {
			return fmt.Errorf("deactivate other chart boards: %w", err)
		}
		return nil
	})
	if err != nil {
		return port.ChartBoardState{}, err
	}
	return s.List(ctx)
}

func (s *ChartBoardStore) Update(ctx context.Context, id uint64, name *string, config *string) (port.ChartBoardState, error) {
	// MySQL 默认不含 CLIENT_FOUND_ROWS：同值 UPDATE 的 RowsAffected 为 0，
	// 因此先做存在性检查再无条件更新，不能依赖 affected 行数判断 NotFound。
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row ChartBoardModel
		if err := tx.Select("id").Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return chartBoardErrNotFound(id)
			}
			return fmt.Errorf("load chart board: %w", err)
		}
		updates := map[string]any{}
		if name != nil {
			updates["name"] = *name
		}
		if config != nil {
			updates["config"] = *config
		}
		if err := tx.Model(&ChartBoardModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return fmt.Errorf("update chart board: %w", err)
		}
		return nil
	})
	if err != nil {
		return port.ChartBoardState{}, err
	}
	return s.List(ctx)
}

func (s *ChartBoardStore) Activate(ctx context.Context, id uint64) (port.ChartBoardState, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row ChartBoardModel
		if err := tx.Select("id").Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return chartBoardErrNotFound(id)
			}
			return fmt.Errorf("load chart board: %w", err)
		}
		if err := tx.Model(&ChartBoardModel{}).Where("id = ?", id).UpdateColumn("is_active", true).Error; err != nil {
			return fmt.Errorf("activate chart board: %w", err)
		}
		if err := tx.Model(&ChartBoardModel{}).Where("id <> ?", id).UpdateColumn("is_active", false).Error; err != nil {
			return fmt.Errorf("deactivate other chart boards: %w", err)
		}
		return nil
	})
	if err != nil {
		return port.ChartBoardState{}, err
	}
	return s.List(ctx)
}

func (s *ChartBoardStore) Delete(ctx context.Context, id uint64) (port.ChartBoardState, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row ChartBoardModel
		if err := tx.Select("id", "is_active").Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return chartBoardErrNotFound(id)
			}
			return fmt.Errorf("load chart board: %w", err)
		}
		if err := tx.Where("id = ?", id).Delete(&ChartBoardModel{}).Error; err != nil {
			return fmt.Errorf("delete chart board: %w", err)
		}
		if !row.IsActive {
			return nil
		}
		var remaining ChartBoardModel
		err := tx.Select("id").Order("id").Take(&remaining).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return nil
		case err != nil:
			return fmt.Errorf("pick next active chart board: %w", err)
		}
		if err := tx.Model(&ChartBoardModel{}).Where("id = ?", remaining.ID).UpdateColumn("is_active", true).Error; err != nil {
			return fmt.Errorf("activate next chart board: %w", err)
		}
		return nil
	})
	if err != nil {
		return port.ChartBoardState{}, err
	}
	return s.List(ctx)
}
