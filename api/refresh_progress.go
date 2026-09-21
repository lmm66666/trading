package api

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"log/slog"
	"strconv"
	"time"
	"trading/internal/application"
	"trading/internal/port"
)

func registerRefreshQueries(r gin.IRoutes, prefix string, h *StockHandler) {
	r.GET(prefix+"/status", h.RefreshStatus)
	r.GET(prefix+"/runs", h.RefreshRuns)
	r.GET(prefix+"/runs/:run_id", h.RefreshRun)
	r.GET(prefix+"/runs/:run_id/failures", h.RefreshFailures)
}
func (h *StockHandler) refreshQueryReady(c *gin.Context) bool {
	if h.kernel.RefreshQueries == nil {
		writeRefreshQueryError(c, "refresh progress", errKernelNotConfigured)
		return false
	}
	return true
}
func refreshQuery(c *gin.Context, cursor string, defaultLimit int, allowKind bool) (string, uint64, int, error) {
	q := c.Request.URL.Query()
	for k, v := range q {
		if len(v) != 1 || (k != "limit" && k != cursor && !(allowKind && k == "kind")) {
			return "", 0, 0, application.ErrInvalidRequest
		}
	}
	kind := q.Get("kind")
	if kind != "" && kind != "STOCK" && kind != "FUTURES" {
		return "", 0, 0, application.ErrInvalidRequest
	}
	limit := defaultLimit
	if values, ok := q["limit"]; ok {
		n, err := strconv.Atoi(values[0])
		if err != nil || n < 1 || n > 100 {
			return "", 0, 0, application.ErrInvalidRequest
		}
		limit = n
	}
	var after uint64
	if values, ok := q[cursor]; ok {
		n, err := strconv.ParseUint(values[0], 10, 64)
		if err != nil {
			return "", 0, 0, application.ErrInvalidRequest
		}
		after = n
	}
	return kind, after, limit, nil
}
func (h *StockHandler) RefreshStatus(c *gin.Context) {
	if !h.refreshQueryReady(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeRefreshQueryError(c, "refresh query", application.ErrInvalidRequest)
		return
	}
	status, err := h.kernel.RefreshQueries.Status(c.Request.Context())
	if err != nil {
		writeRefreshQueryError(c, "refresh status", err)
		return
	}
	status.FuturesEnabled = h.kernel.FuturesEnabled
	if h.kernel.RemoteRefresh != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		status.FuturesEnabled = h.kernel.RemoteRefresh.futuresEnabled(ctx)
	}
	respondSuccess(c, status)
}
func (h *StockHandler) RefreshRuns(c *gin.Context) {
	if !h.refreshQueryReady(c) {
		return
	}
	kind, before, limit, err := refreshQuery(c, "before_id", 20, true)
	if err != nil {
		writeRefreshQueryError(c, "refresh query", err)
		return
	}
	rows, err := h.kernel.RefreshQueries.ListRefreshRuns(c.Request.Context(), kind, before, limit)
	if err != nil {
		writeRefreshQueryError(c, "refresh runs", err)
		return
	}
	respondSuccess(c, gin.H{"items": rows})
}
func (h *StockHandler) RefreshRun(c *gin.Context) {
	if !h.refreshQueryReady(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeRefreshQueryError(c, "refresh query", application.ErrInvalidRequest)
		return
	}
	row, err := h.kernel.RefreshQueries.GetRefreshRun(c.Request.Context(), c.Param("run_id"))
	if err != nil {
		writeRefreshQueryError(c, "refresh run", err)
		return
	}
	respondSuccess(c, row)
}
func (h *StockHandler) RefreshFailures(c *gin.Context) {
	if !h.refreshQueryReady(c) {
		return
	}
	_, after, limit, err := refreshQuery(c, "after_id", 50, false)
	if err != nil {
		writeRefreshQueryError(c, "refresh query", err)
		return
	}
	rows, err := h.kernel.RefreshQueries.ListRefreshFailures(c.Request.Context(), c.Param("run_id"), after, limit)
	if err != nil {
		writeRefreshQueryError(c, "refresh failures", err)
		return
	}
	respondSuccess(c, gin.H{"items": rows})
}

func writeRefreshQueryError(c *gin.Context, op string, err error) {
	if errors.Is(err, application.ErrInvalidRequest) || errors.Is(err, port.ErrInvalidPortValue) || errors.Is(err, port.ErrRefreshRunNotFound) {
		writeApplicationError(c, op, err)
		return
	}
	slog.Error("refresh progress query unavailable", "operation", op)
	respondError(c, 500, "internal server error")
}
