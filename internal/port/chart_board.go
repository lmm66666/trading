package port

import (
	"context"
	"errors"
)

// MaxChartBoards bounds the single-user chart board count.
const MaxChartBoards = 20

// ErrChartBoardNotFound reports an unknown chart board id.
var ErrChartBoardNotFound = errors.New("chart board not found")

// ChartBoard is a persisted chart board; Config holds the normalized JSON
// text produced by the application layer's write path.
type ChartBoard struct {
	ID     uint64
	Name   string
	Config string
}

// ChartBoardState is the full board list plus the active board id.
type ChartBoardState struct {
	Boards   []ChartBoard
	ActiveID uint64
}

// ChartBoardStore persists the single-user chart boards. All mutations
// return the resulting full state; is_active exactly-one is maintained
// inside each transaction.
type ChartBoardStore interface {
	List(ctx context.Context) (ChartBoardState, error)
	Create(ctx context.Context, name, config string) (ChartBoardState, error)
	Update(ctx context.Context, id uint64, name *string, config *string) (ChartBoardState, error)
	Activate(ctx context.Context, id uint64) (ChartBoardState, error)
	Delete(ctx context.Context, id uint64) (ChartBoardState, error)
}
