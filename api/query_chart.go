package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"trading/internal/application"
	"trading/internal/market"
)

type chartQueryRequest struct {
	Comparison  string                         `json:"comparison,omitempty"`
	Instrument  string                         `json:"instrument"`
	Timeframe   string                         `json:"timeframe"`
	PriceView   string                         `json:"price_view"`
	Before      *time.Time                     `json:"before,omitempty"`
	Limit       int                            `json:"limit,omitempty"`
	DataVersion market.DataVersion             `json:"data_version,omitempty"`
	Indicators  []application.IndicatorRequest `json:"indicators,omitempty"`
}

func (h *StockHandler) QueryChart(c *gin.Context) {
	if h.kernel.ChartQueries == nil {
		writeApplicationError(c, "query chart", errKernelNotConfigured)
		return
	}
	var request chartQueryRequest
	if err := strictJSON(c, &request); err != nil {
		writeApplicationError(c, "query chart", err)
		return
	}
	instrument, err := market.ParseInstrumentID(request.Instrument)
	if err != nil {
		writeApplicationError(c, "query chart", err)
		return
	}
	timeframe, ok := chartTimeframe(request.Timeframe)
	if !ok {
		writeApplicationError(c, "query chart", application.ErrInvalidRequest)
		return
	}
	view, ok := chartPriceView(request.PriceView)
	if !ok {
		writeApplicationError(c, "query chart", application.ErrInvalidRequest)
		return
	}
	query := application.ChartQuery{Comparison: request.Comparison, Instrument: instrument, Timeframe: timeframe, View: view, Limit: request.Limit, DataVersion: request.DataVersion, Indicators: request.Indicators}
	if request.Before != nil {
		query.Before = request.Before.UTC().Truncate(time.Microsecond)
	}
	result, err := h.kernel.ChartQueries.Query(c.Request.Context(), query)
	if err != nil {
		writeApplicationError(c, "query chart", err)
		return
	}
	respondSuccess(c, gin.H{
		"instrument":   instrumentSummaryDTO(result.Instrument),
		"timeframe":    chartTimeframeName(result.Timeframe),
		"price_view":   chartPriceViewName(result.View),
		"data_version": result.DataVersion,
		"bars":         result.Bars,
		"series":       result.Series,
		"zscores":      result.ZScores,
		"has_more":     result.HasMore,
		"next_before":  result.NextBefore,
	})
}

func chartTimeframe(value string) (market.Timeframe, bool) {
	switch strings.ToUpper(value) {
	case "DAY":
		return market.Day, true
	case "WEEK":
		return market.Week, true
	default:
		return market.UnknownTimeframe, false
	}
}

func chartTimeframeName(value market.Timeframe) string {
	if value == market.Week {
		return "WEEK"
	}
	return "DAY"
}

func chartPriceView(value string) (market.PriceView, bool) {
	switch strings.ToUpper(value) {
	case "RAW":
		return market.Raw, true
	case "QFQ":
		return market.ForwardAdjusted, true
	default:
		return 0, false
	}
}

func chartPriceViewName(value market.PriceView) string {
	if value == market.ForwardAdjusted {
		return "QFQ"
	}
	return "RAW"
}
