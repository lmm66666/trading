package application

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"sync"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

// Observations never select work or resume collection. A bounded pending set
// tolerates temporary storage outages without retaining an unbounded history.
type RefreshProgress struct {
	recoveryMu sync.Mutex
	recovery   func(context.Context) error
	mu         sync.Mutex
	pending    map[string]*RefreshObservation
	store      port.RefreshProgressWriter
	wake       chan struct{}
}
type RefreshObservation struct {
	mu       sync.Mutex
	saveMu   sync.Mutex
	owner    *RefreshProgress
	snapshot port.RefreshSnapshot
}

func NewRefreshProgress(store port.RefreshProgressWriter) *RefreshProgress {
	return &RefreshProgress{store: store, pending: map[string]*RefreshObservation{}, wake: make(chan struct{}, 1)}
}
func progressNow() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
func (p *RefreshProgress) Begin(ctx context.Context, kind, trigger string) (*RefreshObservation, port.RefreshReceipt) {
	receipt := port.RefreshReceipt{Status: "ACCEPTED", RunID: rand.Text()}
	if p == nil {
		return nil, receipt
	}
	now := progressNow()
	h := &RefreshObservation{owner: p, snapshot: port.RefreshSnapshot{Run: port.RefreshRun{RunID: receipt.RunID, Kind: kind, Trigger: trigger, State: "PREPARING", StartedAt: now, HeartbeatAt: now, SnapshotAt: now, Revision: 1}, Failures: []port.RefreshFailure{}}}
	p.mu.Lock()
	if len(p.pending) >= 64 {
		p.mu.Unlock()
		slog.Warn("refresh progress capacity reached")
		return nil, receipt
	}
	p.pending[receipt.RunID] = h
	p.mu.Unlock()
	receipt.ProgressAvailable = h.save(ctx)
	return h, receipt
}
func (h *RefreshObservation) signal() {
	select {
	case h.owner.wake <- struct{}{}:
	default:
	}
}
func (h *RefreshObservation) Prepared(total int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.snapshot.Run.Total = &total
	h.snapshot.Run.State = "RUNNING"
	h.mu.Unlock()
	h.signal()
}
func (h *RefreshObservation) Completed(id market.InstrumentID, err error) {
	if h == nil || errors.Is(err, context.Canceled) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := progressNow()
	h.snapshot.Run.LastProgressAt = &now
	if err == nil {
		h.snapshot.Run.Succeeded++
	} else {
		h.snapshot.Run.Failed++
		h.snapshot.Failures = append(h.snapshot.Failures, port.RefreshFailure{RunID: h.snapshot.Run.RunID, Exchange: id.Exchange, Code: id.Code, ErrorCode: "REFRESH_FAILED", CompletedAt: now})
	}
}
func (h *RefreshObservation) End(err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	r := &h.snapshot.Run
	now := progressNow()
	r.FinishedAt = &now
	switch {
	case errors.Is(err, context.Canceled):
		r.State = "INTERRUPTED"
		r.ErrorCode = "INTERRUPTED"
	case err != nil:
		r.State = "FAILED"
		r.ErrorCode = "REFRESH_FAILED"
	case r.Failed > 0 && r.Succeeded == 0:
		r.State = "FAILED"
	case r.Failed > 0:
		r.State = "PARTIAL_SUCCEEDED"
	default:
		r.State = "SUCCEEDED"
	}
	h.mu.Unlock()
	h.signal()
}
func (h *RefreshObservation) save(ctx context.Context) bool {
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	h.mu.Lock()
	h.snapshot.Run.Revision++
	h.snapshot.Run.SnapshotAt = progressNow()
	if h.snapshot.Run.Active() {
		h.snapshot.Run.HeartbeatAt = h.snapshot.Run.SnapshotAt
	}
	s := h.snapshot
	s.Failures = append([]port.RefreshFailure{}, s.Failures...)
	h.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := h.owner.store.SaveRefresh(ctx, s); err != nil {
		slog.Warn("refresh progress persistence unavailable")
		return false
	}
	if !s.Run.Active() {
		h.owner.mu.Lock()
		delete(h.owner.pending, s.Run.RunID)
		h.owner.mu.Unlock()
	}
	return true
}
func (p *RefreshProgress) SetRecovery(recover func(context.Context) error) {
	p.recoveryMu.Lock()
	defer p.recoveryMu.Unlock()
	p.recovery = recover
}
func (p *RefreshProgress) recover(ctx context.Context) {
	p.recoveryMu.Lock()
	defer p.recoveryMu.Unlock()
	if p.recovery == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := p.recovery(ctx); err != nil {
		slog.Warn("refresh progress recovery unavailable")
		return
	}
	p.recovery = nil
}
func (p *RefreshProgress) Flush(ctx context.Context) {
	if p == nil {
		return
	}
	p.recover(ctx)
	p.mu.Lock()
	handles := make([]*RefreshObservation, 0, len(p.pending))
	for _, h := range p.pending {
		handles = append(handles, h)
	}
	p.mu.Unlock()
	for _, h := range handles {
		if ctx.Err() != nil {
			return
		}
		h.save(ctx)
	}
}
func (p *RefreshProgress) Run(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.Flush(ctx)
		case <-p.wake:
			p.Flush(ctx)
		}
	}
}
