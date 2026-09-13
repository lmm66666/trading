package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"time"
	"trading/internal/application"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

func (h *StockHandler) GetStockBacktest(c *gin.Context) {
	code := c.Query("code")
	name := c.Query("strategy")
	if (market.InstrumentID{Exchange: market.SSE, Code: code}).Validate() != nil || name == "" {
		writeApplicationError(c, "legacy backtest", application.ErrInvalidRequest)
		return
	}
	if h.kernel.Registry == nil || h.kernel.Instruments == nil {
		writeApplicationError(c, "legacy backtest", errors.New("kernel not configured"))
		return
	}
	definition, err := h.kernel.Registry.Definition(name, "1")
	if err != nil {
		writeApplicationError(c, "legacy strategy", err)
		return
	}
	cycle := timeframeName(definition.PrimaryTimeframe)
	if value := c.Query("cycle"); value != "" && value != cycle {
		writeApplicationError(c, "legacy cycle", application.ErrInvalidRequest)
		return
	}
	ids, err := h.kernel.Instruments.ResolveCode(c.Request.Context(), code)
	if err != nil {
		writeApplicationError(c, "resolve instrument", err)
		return
	}
	if len(ids) == 0 {
		writeApplicationError(c, "resolve instrument", port.ErrMarketDataNotFound)
		return
	}
	if len(ids) > 1 {
		writeApplicationError(c, "resolve instrument", errAmbiguousInstrument)
		return
	}
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		writeApplicationError(c, "legacy identity", err)
		return
	}
	key := hex.EncodeToString(value)
	clock := h.kernel.Clock
	if clock == nil {
		clock = time.Now
	}
	end := clock().UTC().Truncate(time.Microsecond)
	req := application.BacktestRequest{Instrument: ids[0], StrategyID: name, StrategyVersion: "1", IdempotencyKey: key, Start: end.AddDate(-10, 0, 0), End: end, Config: backtest.Config{InitialCash: 10_000_000_000, CashFractionBPS: 10000, CommissionBPS: 3, MinimumCommission: 50_000, StampDutyBPS: 5, SlippageBPS: 5, LotSize: 100, HoldBars: definition.DefaultHoldBars}}
	if err := req.Validate(); err != nil {
		writeApplicationError(c, "legacy request", err)
		return
	}
	if err := port.ValidateIdentity(key, "idempotency key", port.MaxIdempotencyKeyBytes, false); err != nil {
		writeApplicationError(c, "legacy request", err)
		return
	}
	run, err := h.kernel.Backtests.Create(c.Request.Context(), req)
	if err != nil {
		writeApplicationError(c, "create legacy backtest", err)
		return
	}
	timeout := h.kernel.SyncWaitTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	result, err := h.waitBacktest(ctx, run.ID)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		respondAccepted(c, runReference(run))
		return
	}
	if err != nil {
		writeApplicationError(c, "wait backtest", err)
		return
	}
	result["code"] = code
	result["strategy"] = name
	result["cycle"] = cycle
	respondSuccess(c, result)
}
func (h *StockHandler) waitBacktest(ctx context.Context, id string) (gin.H, error) {
	interval := h.kernel.PollInterval
	if interval <= 0 {
		interval = 25 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		run, err := h.kernel.Runs.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if run.Kind != port.RunBacktest {
			return nil, port.ErrRunNotFound
		}
		if run.Status == port.RunSucceeded {
			return h.completeBacktestResponse(ctx, run)
		}
		if run.Status == port.RunFailed || run.Status == port.RunCancelled {
			return nil, errRunNotReady
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
func readAllResults[T any](ctx context.Context, id string, read func(context.Context, string, port.PageRequest) (port.Page[T], error)) ([]T, error) {
	items := make([]T, 0)
	page := port.PageRequest{Limit: 1000}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := read(ctx, id, page)
		if err != nil {
			return nil, err
		}
		if len(items)+len(result.Items) > 20_000 {
			return nil, errors.New("result exceeds synchronous bounds")
		}
		items = append(items, result.Items...)
		if result.NextSequence == nil {
			return items, nil
		}
		if *result.NextSequence <= page.AfterSequence || len(result.Items) == 0 {
			return nil, errors.New("invalid result cursor")
		}
		page.AfterSequence = *result.NextSequence
	}
}
func (h *StockHandler) completeBacktestResponse(ctx context.Context, run port.Run) (gin.H, error) {
	summary, err := h.kernel.Runs.BacktestResult(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	orders, err := readAllResults(ctx, run.ID, h.kernel.Runs.Orders)
	if err != nil {
		return nil, err
	}
	trades, err := readAllResults(ctx, run.ID, h.kernel.Runs.Trades)
	if err != nil {
		return nil, err
	}
	equity, err := readAllResults(ctx, run.ID, h.kernel.Runs.Equity)
	if err != nil {
		return nil, err
	}
	result := runDetails(run)
	result["summary"] = summaryDTO(summary)
	result["orders"] = mapPage(port.Page[backtest.Order]{Items: orders}, orderDTO).Items
	result["trades"] = mapPage(port.Page[backtest.Fill]{Items: trades}, fillDTO).Items
	result["equity"] = mapPage(port.Page[backtest.EquityPoint]{Items: equity}, equityDTO).Items
	return result, nil
}
