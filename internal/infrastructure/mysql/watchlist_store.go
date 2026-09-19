package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"trading/internal/market"
	"trading/internal/port"
)

// WatchlistStore persists the single-user watchlist and reads daily quotes.
type WatchlistStore struct{ db *gorm.DB }

var _ port.WatchlistStore = (*WatchlistStore)(nil)
var _ port.DailyQuoteReader = (*WatchlistStore)(nil)

func NewWatchlistStore(db *gorm.DB) *WatchlistStore { return &WatchlistStore{db: db} }

type watchlistJoinRow struct {
	RowID    uint64 `gorm:"column:row_id"`
	Exchange string `gorm:"column:exchange"`
	Code     string `gorm:"column:code"`
	Name     string `gorm:"column:name"`
	Board    string `gorm:"column:board"`
	LotSize  int64  `gorm:"column:lot_size"`
}

func (s *WatchlistStore) List(ctx context.Context) ([]port.WatchlistEntry, error) {
	var rows []watchlistJoinRow
	err := s.db.WithContext(ctx).
		Table("t_watchlist AS w").
		Select("i.id AS row_id, w.exchange, w.code, i.name, i.board, i.lot_size").
		Joins("JOIN t_instruments i ON i.exchange = w.exchange AND i.code = w.code AND i.active = ?", true).
		Order("w.id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list watchlist: %w", err)
	}
	entries := make([]port.WatchlistEntry, 0, len(rows))
	for _, row := range rows {
		id := market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("stored watchlist instrument: %w", err)
		}
		entries = append(entries, port.WatchlistEntry{
			InstrumentSummary: port.InstrumentSummary{ID: id, Name: row.Name, Board: row.Board, Active: true, LotSize: row.LotSize},
			InstrumentRowID:   row.RowID,
		})
	}
	return entries, nil
}

func (s *WatchlistStore) Add(ctx context.Context, id market.InstrumentID) (bool, error) {
	var exists bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Model(&InstrumentModel{}).Where("exchange = ? AND code = ? AND active = ?", string(id.Exchange), id.Code, true).Count(&active).Error; err != nil {
			return fmt.Errorf("check watchlist instrument: %w", err)
		}
		if active == 0 {
			return fmt.Errorf("%w: instrument %s", port.ErrMarketDataNotFound, id)
		}
		row := WatchlistModel{Exchange: string(id.Exchange), Code: id.Code}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return fmt.Errorf("add watchlist item: %w", result.Error)
		}
		exists = result.RowsAffected == 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *WatchlistStore) Remove(ctx context.Context, id market.InstrumentID) error {
	if err := s.db.WithContext(ctx).Where("exchange = ? AND code = ?", string(id.Exchange), id.Code).Delete(&WatchlistModel{}).Error; err != nil {
		return fmt.Errorf("remove watchlist item: %w", err)
	}
	return nil
}

func (s *WatchlistStore) Count(ctx context.Context) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&WatchlistModel{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count watchlist: %w", err)
	}
	return int(count), nil
}

type dailyQuoteRankRow struct {
	InstrumentID uint64 `gorm:"column:instrument_id"`
	Rank         int    `gorm:"column:rn"`
	Close        int64  `gorm:"column:close"`
}

// LatestDailyQuotes reads each instrument's two most recent current daily
// bars visible at the latest COMPLETE data version and converts them to
// yuan-denominated close/change values.
func (s *WatchlistStore) LatestDailyQuotes(ctx context.Context, ids []uint64) (map[uint64]port.DailyQuote, error) {
	result := make(map[uint64]port.DailyQuote, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var versionRow struct{ Version uint64 }
	err := s.db.WithContext(ctx).Model(&DataVersionModel{}).
		Select("version").
		Where("version > 0 AND status = ?", versionComplete).
		Order("version DESC").
		Limit(1).
		Scan(&versionRow).Error
	if err != nil {
		return nil, fmt.Errorf("latest complete version for quotes: %w", err)
	}
	if versionRow.Version == 0 {
		return result, nil
	}
	var rows []dailyQuoteRankRow
	err = s.db.WithContext(ctx).Raw(`
		SELECT instrument_id, rn, close FROM (
			SELECT instrument_id, close,
			       ROW_NUMBER() OVER (PARTITION BY instrument_id ORDER BY close_time DESC, revision DESC) AS rn
			FROM t_market_bars
			WHERE timeframe = ? AND valid_to_version IS NULL AND valid_from_version <= ? AND instrument_id IN (?)
		) ranked
		WHERE rn <= 2
		ORDER BY instrument_id, rn
	`, timeframeName(market.Day), versionRow.Version, ids).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("latest daily quotes: %w", err)
	}
	for _, row := range rows {
		quote := result[row.InstrumentID]
		close := float64(row.Close) / float64(market.ValueScale)
		if row.Rank == 1 {
			quote.Close = &close
		} else if quote.Close != nil {
			change := *quote.Close - close
			quote.Change = &change
			if close != 0 {
				pct := change / close * 100
				quote.ChangePercent = &pct
			}
		}
		result[row.InstrumentID] = quote
	}
	return result, nil
}
