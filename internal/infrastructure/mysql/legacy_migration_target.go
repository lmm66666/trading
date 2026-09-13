package mysql

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"trading/internal/market"
	"trading/internal/port"
)

type legacyTargetCursor struct {
	InstrumentID uint64
	Digest       string
	Positions    [4]int
	Verified     bool
}

// 首版本迁移不接管已发布的库，也不关闭历史修订。真实 INCOMPLETE
// 版本的行可分批提交；普通发布器在该版本完成前拒绝新发布。
func inspectLegacyTarget(db *gorm.DB) (DataVersionModel, error) {
	var latest DataVersionModel
	if err := db.Order("version DESC").Take(&latest).Error; err != nil {
		return latest, err
	}
	if latest.Version > 0 && (latest.Version != 1 || latest.Source != legacyMigrationSource || (latest.Status != versionComplete && latest.Status != string(port.DataIncomplete))) {
		return latest, ErrLegacyTargetNotEmpty
	}
	if latest.Version > 0 && latest.Status == versionComplete {
		return latest, nil
	}
	for _, model := range []any{&MarketBarModel{}, &AdjustmentFactorModel{}, &CorporateActionModel{}} {
		query := db.Model(model)
		if latest.Version > 0 {
			query = query.Where("valid_from_version <> ?", latest.Version)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return latest, err
		}
		if count > 0 {
			return latest, ErrLegacyTargetNotEmpty
		}
	}
	return latest, nil
}

func prepareLegacyMigration(db *gorm.DB) (MigrationReport, error) {
	report := MigrationReport{Quality: port.DataIncomplete}
	err := db.Transaction(func(tx *gorm.DB) error {
		var anchor DataVersionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = 0 AND status = ? AND source = ?", versionLock, versionLockSource).Take(&anchor).Error; err != nil {
			return err
		}
		latest, err := inspectLegacyTarget(tx)
		if err != nil {
			return err
		}
		if latest.Status == versionComplete && latest.Version > 0 {
			return ErrLegacyTargetNotEmpty
		}
		if latest.Version == 0 {
			row := DataVersionModel{Version: 1, Source: legacyMigrationSource, Status: string(port.DataIncomplete), Quality: string(port.DataIncomplete)}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			latest = row
		}
		report.Version = market.DataVersion(latest.Version)
		found, err := readMigrationState(tx, "report", "", 0, &report)
		if err != nil {
			return err
		}
		if found {
			if report.Version != market.DataVersion(latest.Version) {
				return ErrLegacyChanged
			}
			return nil
		}
		return putMigrationState(tx, "report", "", 0, report)
	})
	return report, err
}

func writeLegacyInstrument(db *gorm.DB, version market.DataVersion, item legacyInstrument, batch port.MarketWriteBatch, size int) error {
	var state legacyTargetCursor
	found, err := readMigrationState(db, "target", item.ID.String(), 0, &state)
	if err != nil {
		return err
	}
	if found && state.Digest != batch.Digest {
		return ErrLegacyChanged
	}
	if state.Verified {
		return nil
	}
	if !found {
		err = db.Transaction(func(tx *gorm.DB) error {
			var instrument InstrumentModel
			err := tx.Where("exchange = ? AND code = ?", string(item.ID.Exchange), item.ID.Code).Take(&instrument).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				instrument = InstrumentModel{Exchange: string(item.ID.Exchange), Code: item.ID.Code, Name: item.Name, Source: legacyMigrationSource}
				if err := tx.Create(&instrument).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			state = legacyTargetCursor{InstrumentID: instrument.ID, Digest: batch.Digest}
			return putMigrationState(tx, "target", item.ID.String(), 0, state)
		})
		if err != nil {
			return err
		}
	}
	lengths := [4]int{len(batch.Bars[market.Day]), len(batch.Bars[market.Week]), len(batch.Factors), len(batch.Actions)}
	for kind, total := range lengths {
		if state.Positions[kind] < 0 || state.Positions[kind] > total {
			return ErrLegacyChanged
		}
		for state.Positions[kind] < total {
			start := state.Positions[kind]
			end := min(start+size, total)
			next := state
			next.Positions[kind] = end
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := insertLegacyTargetChunk(tx, state.InstrumentID, uint64(version), batch, kind, start, end); err != nil {
					return err
				}
				return putMigrationState(tx, "target", item.ID.String(), 0, next)
			})
			if err != nil {
				return err
			}
			state = next
		}
	}
	if err := verifyLegacyInstrument(db, state.InstrumentID, version, batch, size); err != nil {
		return err
	}
	state.Verified = true
	return db.Transaction(func(tx *gorm.DB) error {
		if err := putMigrationState(tx, "target", item.ID.String(), 0, state); err != nil {
			return err
		}
		return putMigrationState(tx, "verified", item.ID.String(), 0, batch.Digest)
	})
}

func insertLegacyTargetChunk(tx *gorm.DB, id, version uint64, batch port.MarketWriteBatch, kind, start, end int) error {
	switch kind {
	case 0, 1:
		tf := market.Day
		if kind == 1 {
			tf = market.Week
		}
		rows := make([]MarketBarModel, 0, end-start)
		for _, bar := range batch.Bars[tf][start:end] {
			rows = append(rows, barModel(id, bar, 1, version))
		}
		return createLegacyTargetRows(tx, rows)
	case 2:
		rows := make([]AdjustmentFactorModel, 0, end-start)
		for _, factor := range batch.Factors[start:end] {
			rows = append(rows, AdjustmentFactorModel{InstrumentID: id, EffectiveTime: factor.EffectiveTime, DataVersion: version, Numerator: factor.Numerator, Denominator: factor.Denominator, ValidFromVersion: version})
		}
		return createLegacyTargetRows(tx, rows)
	case 3:
		rows := make([]CorporateActionModel, 0, end-start)
		for _, action := range batch.Actions[start:end] {
			rows = append(rows, actionModel(id, action, version))
		}
		return createLegacyTargetRows(tx, rows)
	}
	return invalid("unknown legacy target kind")
}

// MySQL 5.7/8.0 每条预处理语句最多绑定 65535 个参数，预留 1024。
// 只拆 SQL，不另开事务：调用方的外部批次及其检查点仍原子提交。
func createLegacyTargetRows[T any](tx *gorm.DB, rows []T) error {
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(&rows); err != nil {
		return err
	}
	columns := 0
	for _, field := range statement.Schema.Fields {
		// 这些新建目标行的自增 ID 均为零，不参与 INSERT；其余可写列
		// 按最大绑定数计入，避免模型新增字段后仍沿用过时的固定行数。
		if field.DBName != "" && field.Creatable && !field.AutoIncrement {
			columns++
		}
	}
	const parameterBudget = 65535 - 1024
	if columns == 0 || columns > parameterBudget {
		return invalid("invalid legacy target column count")
	}
	size := parameterBudget / columns
	for start := 0; start < len(rows); start += size {
		chunk := rows[start:min(start+size, len(rows))]
		if err := tx.Create(&chunk).Error; err != nil {
			return err
		}
	}
	return nil
}

// 校验只加载当前证券；完整摘要包含 count、日期和全部 OHLCV/因子/事件。
// 对目标表的每条读取也受 batch-size 限制，不读取全市场 raw 数据。
func verifyLegacyInstrument(db *gorm.DB, id uint64, version market.DataVersion, expected port.MarketWriteBatch, size int) error {
	actual := port.MarketWriteBatch{Source: legacyMigrationSource, Instrument: expected.Instrument, Bars: map[market.Timeframe][]market.Bar{}}
	for kind := 0; kind < 3; kind++ {
		var cursor uint64
		for {
			query := db.Where("instrument_id = ? AND valid_from_version = ? AND id > ?", id, uint64(version), cursor).Order("id ASC").Limit(size)
			count := 0
			switch kind {
			case 0:
				var rows []MarketBarModel
				if err := query.Find(&rows).Error; err != nil {
					return err
				}
				count = len(rows)
				for _, row := range rows {
					bar, err := row.bar(expected.Instrument, 0)
					if err != nil {
						return err
					}
					actual.Bars[bar.Timeframe] = append(actual.Bars[bar.Timeframe], bar)
					cursor = row.ID
				}
			case 1:
				var rows []AdjustmentFactorModel
				if err := query.Find(&rows).Error; err != nil {
					return err
				}
				count = len(rows)
				for _, row := range rows {
					actual.Factors = append(actual.Factors, market.AdjustmentFactor{EffectiveTime: row.EffectiveTime.UTC(), Numerator: row.Numerator, Denominator: row.Denominator})
					cursor = row.ID
				}
			case 2:
				var rows []CorporateActionModel
				if err := query.Find(&rows).Error; err != nil {
					return err
				}
				count = len(rows)
				for _, row := range rows {
					action, err := row.action(expected.Instrument, 0)
					if err != nil {
						return err
					}
					actual.Actions = append(actual.Actions, action)
					cursor = row.ID
				}
			}
			if count < size {
				break
			}
		}
	}
	digest, err := MarketBatchDigest(actual)
	if err != nil {
		return err
	}
	if digest != expected.Digest {
		return ErrLegacyChanged
	}
	return nil
}

// 验证凭证在每个目标证券写入并重新读取校验后持久化。最终事务只锁定
// 一个真实版本、检查凭证数、更新状态和报告，不搬运或写入市场历史。
func completeLegacyMigration(db *gorm.DB, report *MigrationReport) error {
	candidate := *report
	candidate.Quality = port.DataComplete
	candidate.BacktestEnabled = true
	report.Quality = port.DataIncomplete
	report.BacktestEnabled = false
	err := db.Transaction(func(tx *gorm.DB) error {
		var version DataVersionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = ? AND source = ? AND status = ?", uint64(report.Version), legacyMigrationSource, string(port.DataIncomplete)).Take(&version).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&legacyMigrationRecord{}).Where("stage = ?", "verified").Count(&count).Error; err != nil {
			return err
		}
		if count != report.InstrumentCount || count == 0 || len(report.Failures) > 0 || len(report.RejectedCodes) > 0 {
			return ErrMigrationIncomplete
		}
		result := tx.Model(&DataVersionModel{}).Where("version = ? AND status = ?", uint64(report.Version), string(port.DataIncomplete)).Updates(map[string]any{"status": versionComplete, "quality": string(port.DataComplete), "digest": report.Digest, "published_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLegacyChanged
		}
		return putMigrationState(tx, "report", "", 0, candidate)
	})
	if err == nil {
		*report = candidate
	}
	return err
}
