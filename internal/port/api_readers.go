package port

import (
	"context"
	"trading/internal/market"
)

// InstrumentCodeReader 精确读取活跃证券；返回最多两个匹配，供边界识别歧义。
type InstrumentCodeReader interface {
	ResolveCode(context.Context, string) ([]market.InstrumentID, error)
}

// PublishedSnapshotKeyReader 只读已发布快照身份。可选版本用于旧接口的跨版本
// 最新查询；SnapshotID 非空时必须精确匹配该身份，不能重新选最新记录。
type PublishedSnapshotKeyReader interface {
	LatestPublishedKey(ctx context.Context, strategyID, strategyVersion, snapshotID string) (SnapshotKey, error)
}
