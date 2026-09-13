package mysql

import "time"

// BaseModel uses UTC audit timestamps and has no soft-delete scope: version
// intervals, rather than GORM deletion, govern historical visibility.
type BaseModel struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement;type:bigint unsigned"`
	CreatedAt time.Time `gorm:"type:datetime(6);not null"`
	UpdatedAt time.Time `gorm:"type:datetime(6);not null"`
}
