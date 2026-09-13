package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"trading/internal/application"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

type BacktestRuns interface {
	Create(context.Context, application.BacktestRequest) (port.Run, error)
	Cancel(context.Context, string) error
}
type ScanRuns interface {
	Create(context.Context, application.ScanRequest) (port.Run, error)
	Cancel(context.Context, string) error
	Latest(context.Context, port.SnapshotKey, port.PageRequest) (port.SignalSnapshot, error)
}

// KernelServices 显式注入持久化用例；旧接口不再回退到旧技术策略引擎。
type KernelServices struct {
	Backtests       BacktestRuns
	Scans           ScanRuns
	Runs            port.RunStore
	Registry        *strategy.Registry
	Instruments     port.InstrumentCodeReader
	SnapshotKeys    port.PublishedSnapshotKeyReader
	SyncWaitTimeout time.Duration
	PollInterval    time.Duration
	Clock           func() time.Time
}

func writeApplicationError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, port.ErrIdempotencyConflict):
		respondError(c, 409, "IDEMPOTENCY_CONFLICT")
	case errors.Is(err, port.ErrSnapshotNotReady):
		respondError(c, 409, "SIGNAL_SNAPSHOT_NOT_READY")
	case errors.Is(err, errRunNotReady):
		respondError(c, 409, "RUN_RESULT_NOT_READY")
	case errors.Is(err, errAmbiguousInstrument):
		respondError(c, 409, "AMBIGUOUS_INSTRUMENT")
	case errors.Is(err, application.ErrInvalidRequest), errors.Is(err, application.ErrDateRangeTooLarge), errors.Is(err, port.ErrInvalidPortValue), errors.Is(err, strategy.ErrInvalidParameter), errors.Is(err, strategy.ErrUnknownParameter), errors.Is(err, market.ErrInvalidInstrument), errors.Is(err, market.ErrExchangeRequired), errors.Is(err, backtest.ErrInvalidConfig):
		respondError(c, 400, "INVALID_REQUEST")
	case errors.Is(err, strategy.ErrUnknownStrategy), errors.Is(err, port.ErrRunNotFound), errors.Is(err, port.ErrMarketDataNotFound):
		respondError(c, 404, "NOT_FOUND")
	default:
		respondInternalError(c, op, err)
	}
}

var errRunNotReady = errors.New("run result not ready")
var errAmbiguousInstrument = errors.New("ambiguous instrument")

// strictJSON 限制整个请求体（含空白），拒绝未知字段和第二个 JSON 值。
func strictJSON(c *gin.Context, dst any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return application.ErrInvalidRequest
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return application.ErrInvalidRequest
	}
	return nil
}
func readPage(c *gin.Context) (port.PageRequest, error) {
	p := port.PageRequest{Limit: 100}
	for _, name := range []string{"limit", "after_sequence"} {
		values, exists := c.Request.URL.Query()[name]
		if !exists {
			continue
		}
		if len(values) != 1 {
			return p, application.ErrInvalidRequest
		}
		value, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil {
			return p, application.ErrInvalidRequest
		}
		if name == "limit" {
			if value < 1 || value > 1000 {
				return p, application.ErrInvalidRequest
			}
			p.Limit = int(value)
		} else {
			p.AfterSequence = value
		}
	}
	return p, p.Validate()
}
func runReference(run port.Run) gin.H { return gin.H{"run_id": run.ID, "status": run.Status} }
func runDetails(run port.Run) gin.H {
	return gin.H{"run_id": run.ID, "status": run.Status, "kind": run.Kind, "strategy": run.StrategyID, "strategy_version": run.StrategyVersion, "data_version": run.DataVersion, "engine_version": run.EngineVersion, "attempts": run.Attempts, "cancel_requested_at": run.CancelRequestedAt}
}
func respondAccepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, response{Code: 0, Message: "success", Data: data})
}
func (h *StockHandler) readRun(c *gin.Context, kind port.RunKind, ready bool) (port.Run, error) {
	id := c.Param("run_id")
	if err := port.ValidateIdentity(id, "run ID", port.MaxRunIDBytes, false); err != nil {
		return port.Run{}, err
	}
	if h.kernel.Runs == nil {
		return port.Run{}, errors.New("run store not configured")
	}
	run, err := h.kernel.Runs.Get(c.Request.Context(), id)
	if err != nil {
		return port.Run{}, err
	}
	if run.Kind != kind {
		return port.Run{}, port.ErrRunNotFound
	}
	if ready && run.Status != port.RunSucceeded {
		return port.Run{}, errRunNotReady
	}
	return run, nil
}
func summaryDTO(s backtest.Summary) gin.H {
	return gin.H{"total_return": s.TotalReturn, "annualized_return": s.AnnualizedReturn, "maximum_drawdown": s.MaximumDrawdown, "closed_trades": s.ClosedTrades, "win_rate": s.WinRate, "profit_factor": s.ProfitFactor, "average_holding_bars": s.AverageHoldingBars, "has_open_position": s.HasOpenPosition}
}
func mapPage[A, B any](p port.Page[A], convert func(A) B) port.Page[B] {
	items := make([]B, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, convert(item))
	}
	return port.Page[B]{Items: items, NextSequence: p.NextSequence}
}
func orderDTO(o backtest.Order) gin.H {
	return gin.H{"id": o.ID, "instrument": o.Instrument.String(), "side": o.Side, "quantity": o.Quantity, "created_at": o.CreatedAt.UTC(), "reason": o.Reason, "attempted_at": o.AttemptedAt.UTC(), "final_reason": o.FinalReason}
}
func fillDTO(f backtest.Fill) gin.H {
	return gin.H{"id": f.ID, "order_id": f.OrderID, "instrument": f.Instrument.String(), "side": f.Side, "time": f.Time.UTC(), "price": f.Price, "quantity": f.Quantity, "gross": f.Gross, "commission": f.Commission, "stamp_duty": f.StampDuty, "transfer_fee": f.TransferFee}
}
func equityDTO(p backtest.EquityPoint) gin.H {
	return gin.H{"time": p.Time.UTC(), "equity": p.Equity, "cash": p.Cash, "position_value": p.PositionValue}
}
