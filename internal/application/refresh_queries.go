package application

import (
	"context"
	"errors"
	"trading/internal/port"
)

type RefreshQueries struct{ port.RefreshProgressReader }
type RefreshStatus struct {
	Stock          *port.RefreshRun `json:"stock"`
	Futures        *port.RefreshRun `json:"futures"`
	FuturesEnabled *bool            `json:"futures_enabled"`
}

func NewRefreshQueries(reader port.RefreshProgressReader) *RefreshQueries {
	return &RefreshQueries{reader}
}
func (q *RefreshQueries) Status(ctx context.Context) (RefreshStatus, error) {
	out := RefreshStatus{}
	for _, kind := range []string{"STOCK", "FUTURES"} {
		row, err := q.LatestRefreshRun(ctx, kind)
		if errors.Is(err, port.ErrRefreshRunNotFound) {
			continue
		}
		if err != nil {
			return out, err
		}
		if kind == "STOCK" {
			out.Stock = &row
		} else {
			out.Futures = &row
		}
	}
	return out, nil
}
