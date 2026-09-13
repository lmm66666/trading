package mysql

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"trading/internal/market"
	"trading/internal/port"
	"trading/model"
)

var legacyStages = []string{"info", "daily", "weekly"}

type legacySourceCursor struct {
	LastID uint64
	Count  int64
	Digest string
}

// 源 SQL、暂存比较和新行写入始终只保留当前批次。重启重新流式验证
// 已提交前缀的内容，但不重写该前缀；不保留全市场 Kline 或 JSON。
func streamLegacySource(sourceDB, checkpointDB *gorm.DB, opts MigrationOptions, version market.DataVersion, frozen bool) ([]market.InstrumentID, MigrationReport, error) {
	report := MigrationReport{Version: version, Quality: port.DataIncomplete, LastLegacyIDs: map[string]uint64{}, RejectedCodes: []string{}}
	ids := map[market.InstrumentID]bool{}
	rejected := map[string]bool{}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	err := sourceDB.Transaction(func(readDB *gorm.DB) error {
		for stageIndex, stage := range legacyStages {
			var checkpoint legacySourceCursor
			if !opts.DryRun {
				if _, err := readMigrationState(checkpointDB, "source_cursor", "", uint64(stageIndex), &checkpoint); err != nil {
					return err
				}
				// 已完成源扫描后仅验证，不再接收任何新增暂存行；否则一次
				// 失败的源漂移检测本身会污染检查点，使恢复原快照也无法续跑。
				if frozen {
					checkpoint.LastID = ^uint64(0)
				}
			}
			var cursor uint64
			var count int64
			for {
				page, err := legacySourcePage(readDB, stage, cursor, opts.BatchSize, "")
				if err != nil {
					return err
				}
				for i := range page {
					row := &page[i]
					var code string
					if stage == "info" {
						var info model.StockInfo
						if err := json.Unmarshal(row.Payload, &info); err != nil {
							return err
						}
						code = info.Code
					} else {
						var bar model.StockKline
						if err := json.Unmarshal(row.Payload, &bar); err != nil {
							return err
						}
						code = bar.Code
					}
					id, err := MapLegacyInstrument(code)
					if err != nil {
						rejected[code] = true
					} else {
						ids[id] = true
						row.InstrumentKey = id.String()
						if stage != "info" {
							var old model.StockKline
							if err := json.Unmarshal(row.Payload, &old); err != nil {
								return err
							}
							tf := market.Day
							dates := &report.DailyDates
							if stage == "weekly" {
								tf = market.Week
								dates = &report.WeeklyDates
							}
							bar, err := legacyBar(old, tf)
							if err != nil {
								return err
							}
							if tf == market.Day {
								report.DailyBarCount++
							} else {
								report.WeeklyBarCount++
							}
							if dates.From.IsZero() || bar.CloseTime.Before(dates.From) {
								dates.From = bar.CloseTime
							}
							if bar.CloseTime.After(dates.To) {
								dates.To = bar.CloseTime
							}
						}
					}
					if err := encoder.Encode(row); err != nil {
						return err
					}
				}
				report.InstrumentCount = int64(len(ids))
				report.SourceDigest = hex.EncodeToString(hash.Sum(nil))
				end := cursor
				if len(page) > 0 {
					end = page[len(page)-1].LegacyID
				}
				count += int64(len(page))
				if !opts.DryRun {
					next := legacySourceCursor{LastID: end, Count: count, Digest: report.SourceDigest}
					if err := persistLegacySourcePage(checkpointDB, stage, uint64(stageIndex), cursor, checkpoint.LastID, page, next, opts.BatchSize); err != nil {
						return err
					}
				}
				report.LastLegacyIDs[stage] = end
				cursor = end
				if len(page) < opts.BatchSize {
					break
				}
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, report, err
	}
	ordered := make([]market.InstrumentID, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
	for code := range rejected {
		report.RejectedCodes = append(report.RejectedCodes, code)
	}
	sort.Strings(report.RejectedCodes)
	return ordered, report, nil
}

func legacySourcePage(db *gorm.DB, stage string, after uint64, limit int, code string) ([]legacyMigrationRecord, error) {
	query := db.Where("id > ?", after).Order("id ASC").Limit(limit)
	if code != "" {
		query = query.Where("code = ?", code)
	}
	page := make([]legacyMigrationRecord, 0, limit)
	if stage == "info" {
		var rows []model.StockInfo
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			payload, err := json.Marshal(row)
			if err != nil {
				return nil, err
			}
			page = append(page, legacyMigrationRecord{Stage: stage, LegacyID: uint64(row.ID), Payload: payload})
		}
	} else {
		table := "t_stock_kline_daily"
		if stage == "weekly" {
			table = "t_stock_kline_weekly"
		}
		var rows []model.StockKline
		if err := query.Table(table).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			payload, err := json.Marshal(row)
			if err != nil {
				return nil, err
			}
			page = append(page, legacyMigrationRecord{Stage: stage, LegacyID: uint64(row.ID), Payload: payload})
		}
	}
	return page, nil
}

func persistLegacySourcePage(db *gorm.DB, stage string, index, after, committed uint64, page []legacyMigrationRecord, next legacySourceCursor, size int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("stage = ? AND legacy_id > ?", stage, after)
		if len(page) == size {
			query = query.Where("legacy_id <= ?", next.LastID)
		}
		var existing []legacyMigrationRecord
		if err := query.Order("legacy_id ASC").Limit(size + 1).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > len(page) {
			return ErrLegacyChanged
		}
		byID := map[uint64]legacyMigrationRecord{}
		for _, row := range existing {
			byID[row.LegacyID] = row
		}
		missing := make([]legacyMigrationRecord, 0, len(page))
		for _, row := range page {
			if old, ok := byID[row.LegacyID]; ok {
				if old.InstrumentKey != row.InstrumentKey || !bytes.Equal(old.Payload, row.Payload) {
					return ErrLegacyChanged
				}
				delete(byID, row.LegacyID)
			} else {
				if row.LegacyID <= committed {
					return ErrLegacyChanged
				}
				missing = append(missing, row)
			}
		}
		if len(byID) > 0 {
			return ErrLegacyChanged
		}
		if len(missing) > 0 {
			if err := tx.CreateInBatches(missing, size).Error; err != nil {
				return err
			}
		}
		if next.LastID >= committed {
			return putMigrationState(tx, "source_cursor", "", index, next)
		}
		return nil
	})
}

func loadLegacyInstrument(db *gorm.DB, id market.InstrumentID, opts MigrationOptions) (legacyInstrument, error) {
	item := legacyInstrument{ID: id, Bars: map[market.Timeframe][]model.StockKline{}}
	for _, stage := range legacyStages {
		var cursor uint64
		for {
			var page []legacyMigrationRecord
			var err error
			if opts.DryRun {
				page, err = legacySourcePage(db, stage, cursor, opts.BatchSize, id.Code)
			} else {
				err = db.Where("stage = ? AND instrument_key = ? AND legacy_id > ?", stage, id.String(), cursor).Order("legacy_id ASC").Limit(opts.BatchSize).Find(&page).Error
			}
			if err != nil {
				return item, err
			}
			for _, row := range page {
				cursor = row.LegacyID
				if stage == "info" {
					var info model.StockInfo
					if err := json.Unmarshal(row.Payload, &info); err != nil {
						return item, err
					}
					item.Name = info.Name
				} else {
					var bar model.StockKline
					if err := json.Unmarshal(row.Payload, &bar); err != nil {
						return item, err
					}
					tf := market.Day
					if stage == "weekly" {
						tf = market.Week
					}
					item.Bars[tf] = append(item.Bars[tf], bar)
				}
			}
			if len(page) < opts.BatchSize {
				break
			}
		}
	}
	return item, nil
}

func readMigrationState(db *gorm.DB, stage, key string, id uint64, value any) (bool, error) {
	var row legacyMigrationRecord
	err := db.Where("stage = ? AND instrument_key = ? AND legacy_id = ?", stage, key, id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(row.Payload, value); err != nil {
		return false, err
	}
	return true, nil
}

func putMigrationState(db *gorm.DB, stage, key string, id uint64, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row := legacyMigrationRecord{Stage: stage, InstrumentKey: key, LegacyID: id, Payload: payload}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "stage"}, {Name: "instrument_key"}, {Name: "legacy_id"}}, DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"})}).Create(&row).Error
}
