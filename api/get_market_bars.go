package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

// GetMarketBars exposes versioned daily/weekly bars by canonical instrument ID.
func (h *StockHandler) GetMarketBars(c *gin.Context) {
	if h.kernel.MarketQueries == nil {
		writeApplicationError(c, "market bars", errKernelNotConfigured)
		return
	}
	query := c.Request.URL.Query()
	allowed := map[string]bool{"instrument": true, "timeframe": true, "view": true, "from": true, "to": true, "version": true, "limit": true}
	for name, values := range query {
		if !allowed[name] || len(values) != 1 {
			writeApplicationError(c, "market bars query", application.ErrInvalidRequest)
			return
		}
	}
	instruments := query["instrument"]
	if len(instruments) != 1 {
		writeApplicationError(c, "market bars instrument", application.ErrInvalidRequest)
		return
	}
	id, err := market.ParseInstrumentID(instruments[0])
	if err != nil {
		writeApplicationError(c, "market bars instrument", err)
		return
	}
	timeframe := market.Day
	if value := c.DefaultQuery("timeframe", "daily"); value == "weekly" {
		timeframe = market.Week
	} else if value != "daily" {
		writeApplicationError(c, "market bars timeframe", application.ErrInvalidRequest)
		return
	}
	view := market.Raw
	if value := c.DefaultQuery("view", "raw"); value == "qfq" {
		view = market.ForwardAdjusted
	} else if value != "raw" {
		writeApplicationError(c, "market bars view", application.ErrInvalidRequest)
		return
	}
	version, err := strconv.ParseUint(c.DefaultQuery("version", "0"), 10, 64)
	if err != nil {
		writeApplicationError(c, "market bars version", application.ErrInvalidRequest)
		return
	}
	limit, err := positiveQueryInt(c.DefaultQuery("limit", "100"))
	if err != nil || limit > application.MaxPriceBars {
		writeApplicationError(c, "market bars limit", application.ErrInvalidRequest)
		return
	}
	now := time.Now
	if h.kernel.Clock != nil {
		now = h.kernel.Clock
	}
	to := now().UTC().Truncate(time.Microsecond)
	from := to.AddDate(-(port.MaxBacktestRangeYears - 1), 0, 0)
	if value := c.Query("from"); value != "" {
		from, err = time.Parse(time.DateOnly, value)
		if err != nil {
			writeApplicationError(c, "market bars from", application.ErrInvalidRequest)
			return
		}
		from = from.UTC()
	}
	if value := c.Query("to"); value != "" {
		to, err = time.Parse(time.DateOnly, value)
		if err != nil {
			writeApplicationError(c, "market bars to", application.ErrInvalidRequest)
			return
		}
		to = to.UTC().Add(24*time.Hour - time.Microsecond)
	}
	result, err := h.kernel.MarketQueries.Prices(c.Request.Context(), application.PriceQuery{
		Instrument: id,
		Timeframe:  timeframe,
		View:       view,
		From:       from,
		To:         to,
		Version:    market.DataVersion(version),
		Limit:      limit,
	})
	if err != nil {
		writeApplicationError(c, "market bars", err)
		return
	}
	respondSuccess(c, gin.H{
		"instrument":   result.Instrument.String(),
		"timeframe":    c.DefaultQuery("timeframe", "daily"),
		"view":         c.DefaultQuery("view", "raw"),
		"data_version": result.DataVersion,
		"bars":         result.Bars,
	})
}

func positiveQueryInt(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, application.ErrInvalidRequest
	}
	return n, nil
}
