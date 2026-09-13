package api

import (
	"github.com/gin-gonic/gin"
	"time"
	"trading/internal/application"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

type executionConfigRequest struct {
	InitialCash       market.Money `json:"initial_cash"`
	CashFractionBPS   int64        `json:"cash_fraction_bps"`
	CommissionBPS     int64        `json:"commission_bps"`
	MinimumCommission market.Money `json:"minimum_commission"`
	StampDutyBPS      int64        `json:"stamp_duty_bps"`
	TransferFeeBPS    int64        `json:"transfer_fee_bps"`
	SlippageBPS       int64        `json:"slippage_bps"`
	LotSize           int64        `json:"lot_size"`
	HoldBars          int          `json:"hold_bars"`
}

func (c executionConfigRequest) config() backtest.Config {
	return backtest.Config{InitialCash: c.InitialCash, CashFractionBPS: c.CashFractionBPS, CommissionBPS: c.CommissionBPS, MinimumCommission: c.MinimumCommission, StampDutyBPS: c.StampDutyBPS, TransferFeeBPS: c.TransferFeeBPS, SlippageBPS: c.SlippageBPS, LotSize: c.LotSize, HoldBars: c.HoldBars}
}

type createBacktestRequest struct {
	Instrument      string                 `json:"instrument"`
	Strategy        string                 `json:"strategy"`
	StrategyVersion string                 `json:"strategy_version"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	Parameters      map[string]float64     `json:"parameters,omitempty"`
	Start           time.Time              `json:"start"`
	End             time.Time              `json:"end"`
	Config          executionConfigRequest `json:"config"`
}

func (r createBacktestRequest) Validate() (application.BacktestRequest, error) {
	id, err := market.ParseInstrumentID(r.Instrument)
	if err != nil {
		return application.BacktestRequest{}, application.ErrInvalidRequest
	}
	req := application.BacktestRequest{Instrument: id, StrategyID: r.Strategy, StrategyVersion: r.StrategyVersion, IdempotencyKey: r.IdempotencyKey, Parameters: r.Parameters, Start: r.Start.UTC(), End: r.End.UTC(), Config: r.Config.config()}
	for _, f := range []struct {
		v     string
		limit int
	}{{r.Strategy, port.MaxStrategyIDBytes}, {r.StrategyVersion, port.MaxStrategyVersionBytes}, {r.IdempotencyKey, port.MaxIdempotencyKeyBytes}} {
		if err := port.ValidateIdentity(f.v, "request identity", f.limit, false); err != nil {
			return req, err
		}
	}
	return req, req.Validate()
}
func (h *StockHandler) CreateBacktestRun(c *gin.Context) {
	var body createBacktestRequest
	if err := strictJSON(c, &body); err != nil {
		writeApplicationError(c, "decode backtest", err)
		return
	}
	req, err := body.Validate()
	if err != nil {
		writeApplicationError(c, "validate backtest", err)
		return
	}
	run, err := h.kernel.Backtests.Create(c.Request.Context(), req)
	if err != nil {
		writeApplicationError(c, "create backtest", err)
		return
	}
	respondAccepted(c, runReference(run))
}
