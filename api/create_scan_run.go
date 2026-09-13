package api

import (
	"github.com/gin-gonic/gin"
	"time"
	"trading/internal/application"
	"trading/internal/port"
)

type createScanRequest struct {
	Strategy        string               `json:"strategy"`
	StrategyVersion string               `json:"strategy_version"`
	IdempotencyKey  string               `json:"idempotency_key"`
	Parameters      map[string]float64   `json:"parameters,omitempty"`
	Scope           port.InstrumentScope `json:"scope"`
	From            time.Time            `json:"from"`
	AsOf            time.Time            `json:"as_of"`
}

func (r createScanRequest) Validate() (application.ScanRequest, error) {
	req := application.ScanRequest{StrategyID: r.Strategy, StrategyVersion: r.StrategyVersion, IdempotencyKey: r.IdempotencyKey, Parameters: r.Parameters, Scope: r.Scope, From: r.From.UTC(), AsOf: r.AsOf.UTC()}
	return req, req.Validate()
}
func (h *StockHandler) CreateScanRun(c *gin.Context) {
	var body createScanRequest
	if err := strictJSON(c, &body); err != nil {
		writeApplicationError(c, "decode scan", err)
		return
	}
	req, err := body.Validate()
	if err != nil {
		writeApplicationError(c, "validate scan", err)
		return
	}
	run, err := h.kernel.Scans.Create(c.Request.Context(), req)
	if err != nil {
		writeApplicationError(c, "create scan", err)
		return
	}
	respondAccepted(c, runReference(run))
}
