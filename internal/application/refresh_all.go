package application

import (
	"context"
	"errors"
	"trading/internal/port"
)

// TriggerMarketRefresh reuses category guards; one active category never blocks the other.
func TriggerMarketRefresh(ctx context.Context, stock *MarketScheduler, futures *FuturesScheduler, workers int) (port.BatchRefreshReceipt, error) {
	if stock == nil || workers < 1 || workers > MaxMarketWorkers {
		return port.BatchRefreshReceipt{}, invalidRequest("invalid refresh dependencies or workers")
	}
	if err := ctx.Err(); err != nil {
		return port.BatchRefreshReceipt{}, err
	}
	stockReceipt, err := stock.TriggerTracked(ctx, workers)
	result := port.BatchRefreshReceipt{Stock: refreshOutcome(stockReceipt, err), Futures: port.RefreshReceipt{Status: "DISABLED"}}
	if futures != nil {
		receipt, err := futures.TriggerTracked(ctx)
		result.Futures = refreshOutcome(receipt, err)
	}
	return result, nil
}

func refreshOutcome(receipt port.RefreshReceipt, err error) port.RefreshReceipt {
	if errors.Is(err, ErrRefreshAlreadyRunning) {
		return port.RefreshReceipt{Status: "ALREADY_RUNNING"}
	}
	if err != nil {
		return port.RefreshReceipt{Status: "FAILED", ErrorCode: "REFRESH_UNAVAILABLE"}
	}
	return receipt
}
