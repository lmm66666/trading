package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"sort"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

type SignalSnapshotStore struct{ db *gorm.DB }

func NewSignalSnapshotStore(db *gorm.DB) *SignalSnapshotStore { return &SignalSnapshotStore{db: db} }

var _ port.SignalSnapshotStore = (*SignalSnapshotStore)(nil)

type snapshotFailure struct {
	Instrument market.InstrumentID `json:"instrument"`
	Failure    port.Failure        `json:"failure"`
}

func insertSnapshot(tx *gorm.DB, snapshot port.SignalSnapshot, status port.RunStatus, now time.Time) error {
	failures := make([]snapshotFailure, 0, len(snapshot.Failures))
	for id, f := range snapshot.Failures {
		failures = append(failures, snapshotFailure{id, f})
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Instrument.String() < failures[j].Instrument.String() })
	encoded, err := json.Marshal(failures)
	if err != nil {
		return err
	}
	rows := append([]port.SnapshotRow(nil), snapshot.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Instrument.String() < rows[j].Instrument.String() })
	keys := make([][]any, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, []any{string(row.Instrument.Exchange), row.Instrument.Code})
	}
	ids := make(map[market.InstrumentID]uint64, len(rows))
	if len(keys) > 0 {
		var instruments []InstrumentModel
		if err := tx.Where("(exchange, code) IN ?", keys).Find(&instruments).Error; err != nil {
			return err
		}
		for _, i := range instruments {
			ids[market.InstrumentID{Exchange: market.Exchange(i.Exchange), Code: i.Code}] = i.ID
		}
	}
	asOf := snapshot.Key.AsOf
	if asOf.IsZero() {
		asOf = now
	}
	model := SignalSnapshotModel{SnapshotID: snapshot.ID, RunID: snapshot.RunID, StrategyID: snapshot.Key.StrategyID, StrategyVersion: snapshot.Key.StrategyVersion, ParametersHash: snapshot.Key.ParametersHash, DataVersion: uint64(snapshot.DataVersion), AsOf: asOf, Status: string(status), SuccessCount: uint64(len(rows)), FailureCount: uint64(len(failures)), FailuresJSON: encoded}
	if err := tx.Create(&model).Error; err != nil {
		return err
	}
	return batches(len(rows), func(start, end int) error {
		models := make([]SignalSnapshotRowModel, 0, end-start)
		for index := start; index < end; index++ {
			row := rows[index]
			id, ok := ids[row.Instrument]
			if !ok {
				return invalid("unknown snapshot instrument")
			}
			values, err := json.Marshal(row.Values)
			if err != nil {
				return err
			}
			models = append(models, SignalSnapshotRowModel{SnapshotID: snapshot.ID, InstrumentID: id, Sequence: uint64(index + 1), SignalTime: row.SignalTime, Reason: row.Reason, ValuesJSON: values})
		}
		return tx.Create(&models).Error
	})
}
func batches(length int, write func(int, int) error) error {
	for start := 0; start < length; start += 1000 {
		end := start + 1000
		if end > length {
			end = length
		}
		if err := write(start, end); err != nil {
			return err
		}
	}
	return nil
}
func (s *SignalSnapshotStore) Latest(ctx context.Context, key port.SnapshotKey, page port.PageRequest) (port.SignalSnapshot, error) {
	if err := key.Validate(); err != nil {
		return port.SignalSnapshot{}, err
	}
	if err := page.Validate(); err != nil {
		return port.SignalSnapshot{}, err
	}
	var model SignalSnapshotModel
	query := s.db.WithContext(ctx).Where("strategy_id = ? AND strategy_version = ? AND parameters_hash = ? AND status IN ?", key.StrategyID, key.StrategyVersion, key.ParametersHash, []string{string(port.RunSucceeded), string(port.RunPartialSucceeded)})
	if !key.AsOf.IsZero() {
		query = query.Where("as_of = ?", key.AsOf)
	}
	if err := query.Order("as_of DESC, data_version DESC, id DESC").Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return port.SignalSnapshot{}, port.ErrSnapshotNotReady
		}
		return port.SignalSnapshot{}, err
	}
	result := port.SignalSnapshot{ID: model.SnapshotID, RunID: model.RunID, Key: port.SnapshotKey{StrategyID: model.StrategyID, StrategyVersion: model.StrategyVersion, ParametersHash: model.ParametersHash, AsOf: model.AsOf.UTC()}, DataVersion: market.DataVersion(model.DataVersion), Rows: []port.SnapshotRow{}, Failures: map[market.InstrumentID]port.Failure{}}
	var failures []snapshotFailure
	if err := json.Unmarshal(model.FailuresJSON, &failures); err != nil {
		return port.SignalSnapshot{}, err
	}
	for _, f := range failures {
		result.Failures[f.Instrument] = f.Failure
	}
	var rows []struct {
		SignalSnapshotRowModel
		Exchange string
		Code     string
	}
	if err := s.db.WithContext(ctx).Table("t_signal_snapshot_rows AS r").Select("r.*, i.exchange, i.code").Joins("JOIN t_instruments AS i ON i.id = r.instrument_id").Where("r.snapshot_id = ? AND r.sequence > ?", model.SnapshotID, page.AfterSequence).Order("r.sequence").Limit(page.Limit).Scan(&rows).Error; err != nil {
		return port.SignalSnapshot{}, err
	}
	for _, row := range rows {
		value := port.SnapshotRow{Instrument: market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}, SignalTime: row.SignalTime.UTC(), Reason: row.Reason}
		if err := json.Unmarshal(row.ValuesJSON, &value.Values); err != nil {
			return port.SignalSnapshot{}, err
		}
		result.Rows = append(result.Rows, value)
	}
	if err := result.Validate(); err != nil {
		return port.SignalSnapshot{}, err
	}
	return result, nil
}
