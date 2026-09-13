package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"math"
	"sort"
	"time"
	"trading/internal/application"
	"trading/internal/port"
	"trading/internal/strategy"
)

func (h *StockHandler) snapshotKey(c *gin.Context, page port.PageRequest) (port.SnapshotKey, error) {
	id := c.Query("strategy")
	version := c.Query("strategy_version")
	snapshotID := c.Query("snapshot_id")
	for _, f := range []struct {
		v   string
		max int
	}{{version, port.MaxStrategyVersionBytes}, {snapshotID, port.MaxSnapshotIDBytes}, {c.Query("parameters_hash"), port.MaxHashBytes}} {
		if err := port.ValidateIdentity(f.v, "snapshot selector", f.max, true); err != nil {
			return port.SnapshotKey{}, err
		}
	}
	if err := port.ValidateIdentity(id, "strategy", port.MaxStrategyIDBytes, false); err != nil {
		return port.SnapshotKey{}, err
	}
	if page.AfterSequence > 0 && snapshotID == "" {
		return port.SnapshotKey{}, application.ErrInvalidRequest
	}
	if h.kernel.Registry == nil || h.kernel.SnapshotKeys == nil {
		return port.SnapshotKey{}, errors.New("snapshot dependencies not configured")
	}
	found := false
	for _, d := range h.kernel.Registry.Definitions() {
		if d.ID == id && (version == "" || version == d.Version) {
			found = true
			break
		}
	}
	if !found {
		return port.SnapshotKey{}, strategy.ErrUnknownStrategy
	}
	var key port.SnapshotKey
	if hash := c.Query("parameters_hash"); hash != "" && version != "" {
		key = port.SnapshotKey{StrategyID: id, StrategyVersion: version, ParametersHash: hash, SnapshotID: snapshotID}
	} else {
		var err error
		key, err = h.kernel.SnapshotKeys.LatestPublishedKey(c.Request.Context(), id, version, snapshotID)
		if err != nil {
			return key, err
		}
		if hash != "" && hash != key.ParametersHash {
			return key, port.ErrSnapshotNotReady
		}
	}
	if value := c.Query("as_of"); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return key, application.ErrInvalidRequest
		}
		key.AsOf = parsed.UTC().Truncate(time.Microsecond)
	}
	return key, key.Validate()
}
func snapshotDTO(snapshot port.SignalSnapshot, page port.PageRequest) gin.H {
	rows := make([]gin.H, 0, len(snapshot.Rows))
	for _, r := range snapshot.Rows {
		rows = append(rows, gin.H{"instrument": r.Instrument.String(), "signal_time": r.SignalTime.UTC(), "reason": r.Reason, "values": r.Values})
	}
	failures := make([]gin.H, 0, len(snapshot.Failures))
	for id, f := range snapshot.Failures {
		failures = append(failures, gin.H{"instrument": id.String(), "code": f.Code, "message": f.Message, "retryable": f.Retryable})
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i]["instrument"].(string) < failures[j]["instrument"].(string) })
	data := gin.H{"snapshot_id": snapshot.ID, "run_id": snapshot.RunID, "key": snapshot.Key, "data_version": snapshot.DataVersion, "rows": rows, "failures": failures}
	// Task12 的不可变行以 1 开始连续编号。满页给出排他游标；最后一页恰好
	// 满页时允许一次空页，客户端据此结束，不重新选择最新快照。
	if len(rows) == page.Limit && page.AfterSequence <= math.MaxInt64-int64(len(rows)) {
		data["next_sequence"] = page.AfterSequence + int64(len(rows))
	}
	return data
}
func (h *StockHandler) GetLatestSignalSnapshot(c *gin.Context) {
	page, err := readPage(c)
	if err != nil {
		writeApplicationError(c, "snapshot page", err)
		return
	}
	key, err := h.snapshotKey(c, page)
	if err != nil {
		writeApplicationError(c, "snapshot key", err)
		return
	}
	snapshot, err := h.kernel.Scans.Latest(c.Request.Context(), key, page)
	if err != nil {
		writeApplicationError(c, "latest snapshot", err)
		return
	}
	respondSuccess(c, snapshotDTO(snapshot, page))
}
