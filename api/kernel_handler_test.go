package api

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"trading/internal/application"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
	"trading/internal/strategy/builtin"
)

type apiRunStore struct {
	port.RunStore
	run       port.Run
	err       error
	page      port.PageRequest
	ctx       context.Context
	resultErr error
	pageErr   error
}

func (s *apiRunStore) Get(ctx context.Context, id string) (port.Run, error) {
	s.ctx = ctx
	return s.run, s.err
}
func (s *apiRunStore) BacktestResult(ctx context.Context, id string) (backtest.Summary, error) {
	return backtest.Summary{ClosedTrades: 2}, s.resultErr
}
func (s *apiRunStore) Orders(ctx context.Context, id string, p port.PageRequest) (port.Page[backtest.Order], error) {
	if p.AfterSequence >= 7 {
		return port.Page[backtest.Order]{}, s.err
	}
	s.ctx = ctx
	s.page = p
	n := int64(7)
	return port.Page[backtest.Order]{Items: []backtest.Order{{ID: "order-1", Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600000"}}}, NextSequence: &n}, s.pageErr
}
func (s *apiRunStore) Trades(ctx context.Context, id string, p port.PageRequest) (port.Page[backtest.Fill], error) {
	s.ctx = ctx
	s.page = p
	return port.Page[backtest.Fill]{Items: []backtest.Fill{{ID: "fill-1"}}}, s.pageErr
}
func (s *apiRunStore) Equity(ctx context.Context, id string, p port.PageRequest) (port.Page[backtest.EquityPoint], error) {
	s.ctx = ctx
	s.page = p
	return port.Page[backtest.EquityPoint]{Items: []backtest.EquityPoint{{Equity: 12345678901234567}}}, s.pageErr
}

type apiBacktests struct {
	req              application.BacktestRequest
	ctx              context.Context
	run              port.Run
	err              error
	creates, cancels int
}

func (s *apiBacktests) Create(ctx context.Context, r application.BacktestRequest) (port.Run, error) {
	s.ctx = ctx
	s.req = r
	s.creates++
	return s.run, s.err
}
func (s *apiBacktests) Cancel(ctx context.Context, id string) error {
	s.ctx = ctx
	s.cancels++
	return s.err
}

type apiScans struct {
	req              application.ScanRequest
	ctx              context.Context
	run              port.Run
	err              error
	creates, cancels int
	key              port.SnapshotKey
	page             port.PageRequest
	snapshot         port.SignalSnapshot
	latest           func(context.Context, port.SnapshotKey, port.PageRequest) (port.SignalSnapshot, error)
}

func (s *apiScans) Create(ctx context.Context, r application.ScanRequest) (port.Run, error) {
	s.ctx = ctx
	s.req = r
	s.creates++
	return s.run, s.err
}
func (s *apiScans) Cancel(ctx context.Context, id string) error {
	s.ctx = ctx
	s.cancels++
	return s.err
}
func (s *apiScans) Latest(ctx context.Context, k port.SnapshotKey, p port.PageRequest) (port.SignalSnapshot, error) {
	if s.latest != nil {
		return s.latest(ctx, k, p)
	}
	s.ctx = ctx
	s.key = k
	s.page = p
	return s.snapshot, s.err
}

type apiLookup struct {
	ids  []market.InstrumentID
	key  port.SnapshotKey
	err  error
	code string
}

func (s *apiLookup) ResolveCode(ctx context.Context, code string) ([]market.InstrumentID, error) {
	s.code = code
	return s.ids, s.err
}
func (s *apiLookup) LatestPublishedKey(ctx context.Context, id, version, snapshotID string) (port.SnapshotKey, error) {
	return s.key, s.err
}

type kernelFixture struct {
	router   *gin.Engine
	b        *apiBacktests
	s        *apiScans
	store    *apiRunStore
	lookup   *apiLookup
	services KernelServices
}

func newKernelFixture(t *testing.T) *kernelFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	run := port.Run{ID: "run-1", Kind: port.RunBacktest, Status: port.RunPending, StrategyID: "daily_b1_buy", StrategyVersion: "1", DataVersion: 5, EngineVersion: "test", LeaseToken: "secret-lease", RequestJSON: []byte(`{"secret":"db"}`)}
	key := port.SnapshotKey{SnapshotID: "snap-1", StrategyID: "daily_b1_buy", StrategyVersion: "1", ParametersHash: "hash-1"}
	f := &kernelFixture{b: &apiBacktests{run: run}, s: &apiScans{run: run, snapshot: port.SignalSnapshot{ID: "snap-1", RunID: "scan-1", Key: key, DataVersion: 5, Rows: []port.SnapshotRow{{Instrument: market.InstrumentID{Exchange: market.SSE, Code: "600000"}, SignalTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Reason: "hit"}}, Failures: map[market.InstrumentID]port.Failure{{Exchange: market.SZSE, Code: "000001"}: {Code: "DATA", Message: "safe"}}}}, store: &apiRunStore{run: run}, lookup: &apiLookup{ids: []market.InstrumentID{{Exchange: market.SSE, Code: "600000"}}, key: key}}
	registry := &strategy.Registry{}
	require.NoError(t, builtin.RegisterAll(registry))
	f.services = KernelServices{Backtests: f.b, Scans: f.s, Runs: f.store, Registry: registry, Instruments: f.lookup, SnapshotKeys: f.lookup, SyncWaitTimeout: 20 * time.Millisecond, PollInterval: time.Millisecond, Clock: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }}
	f.router = NewRouter(nil, nil, nil, nil, nil, nil, nil, f.services)
	return f
}
func kernelRequest(t *testing.T, f *kernelFixture, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(w, req)
	return w
}

var internalAPIError = errors.New("dial user:secret@db/private_table")

func TestKernelErrorsRedactedAndMapped(t *testing.T) {
	for _, tt := range []struct {
		err     error
		status  int
		message string
	}{{application.ErrInvalidRequest, 400, "INVALID_REQUEST"}, {port.ErrIdempotencyConflict, 409, "IDEMPOTENCY_CONFLICT"}, {strategy.ErrUnknownStrategy, 404, "NOT_FOUND"}, {port.ErrRunNotFound, 404, "NOT_FOUND"}, {port.ErrSnapshotNotReady, 409, "SIGNAL_SNAPSHOT_NOT_READY"}, {internalAPIError, 500, "internal server error"}} {
		t.Run(tt.message, func(t *testing.T) {
			f := newKernelFixture(t)
			f.b.err = tt.err
			w := kernelRequest(t, f, "POST", "/api/v1/backtest-runs", validBacktestBody)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tt.message)
			require.NotContains(t, w.Body.String(), "secret")
		})
	}
}
