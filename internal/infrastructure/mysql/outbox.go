package mysql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"trading/internal/port"
)

type Outbox struct{ db *gorm.DB }

func NewOutbox(db *gorm.DB) *Outbox { return &Outbox{db: db} }

var _ port.EventBus = (*Outbox)(nil)

// Payload is an opaque byte slice at the port. A JSON base64 string stores all
// bytes losslessly, including payloads that are not themselves JSON.
func outboxModel(event port.Event) OutboxEventModel {
	payload, _ := json.Marshal(event.Payload)
	return OutboxEventModel{EventID: event.ID, Kind: event.Kind, AggregateID: event.AggregateID, Payload: payload, OccurredAt: event.OccurredAt.UTC().Truncate(time.Microsecond)}
}
func (b *Outbox) Publish(ctx context.Context, event port.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if len(event.Kind) > 64 {
		return invalid("event kind exceeds 64 bytes")
	}
	row := outboxModel(event)
	return retryTransaction(ctx, b.db, func(tx *gorm.DB) error {
		inserted := row
		if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"event_id": gorm.Expr("event_id")})}).Create(&inserted).Error; err != nil {
			return err
		}
		var existing OutboxEventModel
		if err := tx.Where("event_id = ?", event.ID).Take(&existing).Error; err != nil {
			return err
		}
		var left, right []byte
		if json.Unmarshal(existing.Payload, &left) != nil || json.Unmarshal(row.Payload, &right) != nil || !bytes.Equal(left, right) || existing.Kind != row.Kind || existing.AggregateID != row.AggregateID || !existing.OccurredAt.Equal(row.OccurredAt) {
			return invalid("event ID belongs to a different event")
		}
		return nil
	})
}
func databaseNow(tx *gorm.DB) (time.Time, error) {
	var now time.Time
	err := tx.Raw("SELECT UTC_TIMESTAMP(6)").Row().Scan(&now)
	return now.UTC(), err
}
func completionEvent(tx *gorm.DB, row ComputeRunModel, status port.RunStatus, snapshotID string, now time.Time) error {
	hash := sha256.Sum256([]byte("compute-terminal\x00" + row.RunID))
	payload, _ := json.Marshal(struct {
		RunID      string         `json:"run_id"`
		Status     port.RunStatus `json:"status"`
		SnapshotID string         `json:"snapshot_id,omitempty"`
	}{row.RunID, status, snapshotID})
	event := outboxModel(port.Event{ID: hex.EncodeToString(hash[:]), Kind: "compute.completed", AggregateID: row.RunID, Payload: payload, OccurredAt: now})
	return tx.Create(&event).Error
}
