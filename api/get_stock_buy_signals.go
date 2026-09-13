package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"trading/internal/port"
)

// GetStockBuySignals 只读取最新已发布快照，响应时才去掉交易所。
func (h *StockHandler) GetStockBuySignals(c *gin.Context) {
	page := port.PageRequest{Limit: 1000}
	key, err := h.snapshotKey(c, page)
	if err != nil {
		writeApplicationError(c, "legacy snapshot key", err)
		return
	}
	codes := make([]string, 0)
	for {
		snapshot, err := h.kernel.Scans.Latest(c.Request.Context(), key, page)
		if err != nil {
			writeApplicationError(c, "legacy snapshot", err)
			return
		}
		key = snapshot.Key
		key.SnapshotID = snapshot.ID
		if len(codes)+len(snapshot.Rows) > port.MaxScanInstruments {
			writeApplicationError(c, "legacy snapshot", errors.New("snapshot exceeds instrument bound"))
			return
		}
		for _, row := range snapshot.Rows {
			codes = append(codes, row.Instrument.Code)
		}
		if len(snapshot.Rows) < page.Limit {
			break
		}
		page.AfterSequence += int64(len(snapshot.Rows))
	}
	respondSuccess(c, gin.H{"name": key.StrategyID, "codes": codes})
}
