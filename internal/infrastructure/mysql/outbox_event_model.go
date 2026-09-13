package mysql

import "time"

type OutboxEventModel struct {
	BaseModel
	EventID     string     `gorm:"size:64;not null;uniqueIndex:uq_outbox_event"`
	Kind        string     `gorm:"size:64;not null"`
	AggregateID string     `gorm:"size:128;not null"`
	Payload     []byte     `gorm:"type:json;not null"`
	OccurredAt  time.Time  `gorm:"type:datetime(6);not null"`
	PublishedAt *time.Time `gorm:"type:datetime(6);index:idx_outbox_pending"`
	Attempts    uint32     `gorm:"type:int unsigned;not null"`
}

func (OutboxEventModel) TableName() string { return "t_outbox_events" }
