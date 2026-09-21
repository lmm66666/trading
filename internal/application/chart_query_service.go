package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/port"
)

const (
	DefaultChartLimit     = 400
	MinChartLimit         = 100
	MaxChartLimit         = 1000
	MaxChartIndicators    = 16
	MaxIndicatorPeriod    = 500
	MaxChartIndicatorCost = 2000
	maxChartComputations  = 4
)

type IndicatorKind string

const (
	IndicatorSTD  IndicatorKind = "STD"
	IndicatorSMA  IndicatorKind = "SMA"
	IndicatorEMA  IndicatorKind = "EMA"
	IndicatorMACD IndicatorKind = "MACD"
	IndicatorKDJ  IndicatorKind = "KDJ"
	IndicatorRETZ IndicatorKind = "RETZ"
)

type IndicatorRequest struct {
	Kind   IndicatorKind `json:"kind"`
	Period int           `json:"period,omitempty"`
	Fast   int           `json:"fast,omitempty"`
	Slow   int           `json:"slow,omitempty"`
	Signal int           `json:"signal,omitempty"`
	Smooth int           `json:"smooth,omitempty"`
	Regime int           `json:"regime,omitempty"`
}

type ChartQuery struct {
	Instrument  market.InstrumentID
	Timeframe   market.Timeframe
	View        market.PriceView
	Before      time.Time
	Limit       int
	DataVersion market.DataVersion
	Indicators  []IndicatorRequest
}

type ChartPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

type ChartSeries struct {
	Key       string        `json:"key"`
	Kind      IndicatorKind `json:"kind"`
	Component string        `json:"component"`
	Points    []ChartPoint  `json:"points"`
}

type ChartResult struct {
	Instrument  port.InstrumentSummary
	Timeframe   market.Timeframe
	View        market.PriceView
	DataVersion market.DataVersion
	Bars        []PriceBar
	Series      []ChartSeries
	HasMore     bool
	NextBefore  *time.Time
}

type ChartQueryService struct {
	data    port.MarketData
	catalog port.InstrumentCatalog
	clock   func() time.Time
	compute chan struct{}
}

func NewChartQueryService(data port.MarketData, catalog port.InstrumentCatalog) (*ChartQueryService, error) {
	if data == nil || catalog == nil {
		return nil, invalidRequest("chart query dependencies are required")
	}
	return &ChartQueryService{data: data, catalog: catalog, clock: time.Now, compute: make(chan struct{}, maxChartComputations)}, nil
}

func (s *ChartQueryService) Query(ctx context.Context, query ChartQuery) (ChartResult, error) {
	if err := validateChartQuery(&query); err != nil {
		return ChartResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ChartResult{}, err
	}
	instrument, err := s.catalog.Get(ctx, query.Instrument)
	if err != nil {
		return ChartResult{}, err
	}
	if instrument.ID != query.Instrument || !instrument.Active {
		return ChartResult{}, ErrIncompleteMarketData
	}
	if query.DataVersion == 0 {
		query.DataVersion, err = s.data.LatestCompleteVersion(ctx)
		if err != nil {
			return ChartResult{}, err
		}
	}
	if query.DataVersion == 0 {
		return ChartResult{}, ErrIncompleteMarketData
	}
	to := query.Before
	if to.IsZero() {
		to = s.clock().UTC().Truncate(time.Microsecond)
	} else {
		to = to.Add(-time.Microsecond)
	}
	from := to.AddDate(-MaxBacktestRangeYears, 0, 0)
	dataset, factors, _, err := s.data.Dataset(ctx, query.Instrument, query.Timeframe, from, to, query.DataVersion)
	if err != nil {
		return ChartResult{}, err
	}
	if dataset.Instrument() != query.Instrument || dataset.Timeframe() != query.Timeframe || dataset.Version() != query.DataVersion {
		return ChartResult{}, ErrIncompleteMarketData
	}
	if query.View == market.ForwardAdjusted {
		factors, err = normalizeFactors(factors)
		if err != nil {
			return ChartResult{}, err
		}
	}
	start := dataset.Len() - query.Limit
	if start < 0 {
		start = 0
	}
	result := ChartResult{Instrument: instrument, Timeframe: query.Timeframe, View: query.View, DataVersion: query.DataVersion, Bars: make([]PriceBar, 0, dataset.Len()-start), Series: []ChartSeries{}, HasMore: start > 0}
	factorIndex := 0
	for index := start; index < dataset.Len(); index++ {
		bar, err := chartPriceBar(dataset.Bar(index), query.View, factors, &factorIndex)
		if err != nil {
			return ChartResult{}, err
		}
		result.Bars = append(result.Bars, bar)
	}
	if result.HasMore && len(result.Bars) > 0 {
		next := result.Bars[0].CloseTime
		result.NextBefore = &next
	}
	refs, descriptors := chartIndicatorRefs(query, query.Indicators)
	if len(refs) == 0 {
		return result, nil
	}
	select {
	case s.compute <- struct{}{}:
		defer func() { <-s.compute }()
	case <-ctx.Done():
		return ChartResult{}, ctx.Err()
	}
	features, err := indicator.BuildContext(ctx, dataset, factors, refs)
	if err != nil {
		return ChartResult{}, fmt.Errorf("build chart indicators: %w", err)
	}
	for _, descriptor := range descriptors {
		series := ChartSeries{Key: descriptor.ref.Key(), Kind: descriptor.kind, Component: descriptor.component, Points: make([]ChartPoint, 0, dataset.Len()-start)}
		values := features[descriptor.ref.Key()]
		for index := start; index < dataset.Len(); index++ {
			value, valid := values.At(index)
			if valid {
				series.Points = append(series.Points, ChartPoint{Time: dataset.Bar(index).CloseTime.UTC(), Value: value})
			}
		}
		result.Series = append(result.Series, series)
	}
	return result, nil
}

func validateChartQuery(query *ChartQuery) error {
	if query.Instrument.Validate() != nil || (query.Timeframe != market.Day && query.Timeframe != market.Week) || (query.View != market.Raw && query.View != market.ForwardAdjusted) {
		return invalidRequest("invalid chart identity, timeframe or price view")
	}
	if !query.Before.IsZero() && (query.Before.Location() != time.UTC || query.Before.Nanosecond()%1000 != 0) {
		return invalidRequest("chart cursor must be UTC with microsecond precision")
	}
	if query.Limit == 0 {
		query.Limit = DefaultChartLimit
	}
	if query.Limit < MinChartLimit || query.Limit > MaxChartLimit || len(query.Indicators) > MaxChartIndicators {
		return invalidRequest("chart query exceeds bounds")
	}
	seen := make(map[string]bool, len(query.Indicators))
	cost := 0
	for index := range query.Indicators {
		request := &query.Indicators[index]
		request.Kind = IndicatorKind(strings.ToUpper(string(request.Kind)))
		if err := validateIndicatorRequest(*request); err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%d/%d/%d/%d/%d/%d", request.Kind, request.Period, request.Fast, request.Slow, request.Signal, request.Smooth, request.Regime)
		if seen[key] {
			return invalidRequest("duplicate chart indicator")
		}
		seen[key] = true
		cost += chartIndicatorCost(*request)
		if cost > MaxChartIndicatorCost {
			return invalidRequest("chart indicator cost exceeds bounds")
		}
	}
	return nil
}

func chartIndicatorCost(request IndicatorRequest) int {
	switch request.Kind {
	case IndicatorSTD:
		return request.Period
	case IndicatorKDJ:
		return request.Period * 3
	case IndicatorRETZ:
		return request.Period + request.Regime
	case IndicatorMACD:
		return 3
	default:
		return 1
	}
}

func validateIndicatorRequest(request IndicatorRequest) error {
	if request.Kind != IndicatorRETZ && (request.Smooth != 0 || request.Regime != 0) {
		return invalidRequest("unexpected RETZ parameters")
	}
	switch request.Kind {
	case IndicatorSMA, IndicatorEMA, IndicatorSTD:
		if request.Period < 1 || request.Period > MaxIndicatorPeriod || request.Fast != 0 || request.Slow != 0 || request.Signal != 0 {
			return invalidRequest("invalid moving average parameters")
		}
	case IndicatorMACD:
		if request.Period != 0 || request.Fast < 1 || request.Slow <= request.Fast || request.Slow > MaxIndicatorPeriod || request.Signal < 1 || request.Signal > MaxIndicatorPeriod {
			return invalidRequest("invalid MACD parameters")
		}
	case IndicatorKDJ:
		if request.Period < 1 || request.Period > MaxIndicatorPeriod || request.Fast != 0 || request.Slow != 0 || request.Signal != 0 {
			return invalidRequest("invalid KDJ parameters")
		}
	case IndicatorRETZ:
		if request.Period < 2 || request.Period > MaxIndicatorPeriod || request.Smooth < 1 || request.Smooth > MaxIndicatorPeriod || request.Regime < 2 || request.Regime > MaxIndicatorPeriod || request.Regime <= request.Period || request.Fast != 0 || request.Slow != 0 || request.Signal != 0 {
			return invalidRequest("invalid RETZ parameters")
		}
	default:
		return invalidRequest("unsupported chart indicator")
	}
	return nil
}

func chartPriceBar(bar market.Bar, view market.PriceView, factors []market.AdjustmentFactor, factorIndex *int) (PriceBar, error) {
	multiplier := 1.0 / float64(market.ValueScale)
	if view == market.ForwardAdjusted {
		factor, ok := factorAt(factors, factorIndex, bar.CloseTime)
		if !ok {
			return PriceBar{}, ErrIncompleteMarketData
		}
		multiplier *= float64(factor.Numerator) / float64(factor.Denominator)
	}
	return PriceBar{OpenTime: bar.OpenTime.UTC(), CloseTime: bar.CloseTime.UTC(), Open: float64(bar.Open) * multiplier, High: float64(bar.High) * multiplier, Low: float64(bar.Low) * multiplier, Close: float64(bar.Close) * multiplier, Volume: bar.Volume, Amount: float64(bar.Amount) / float64(market.ValueScale), Trading: bar.Trading}, nil
}

type chartIndicatorDescriptor struct {
	ref       indicator.Ref
	kind      IndicatorKind
	component string
}

func chartIndicatorRefs(query ChartQuery, requests []IndicatorRequest) ([]indicator.Ref, []chartIndicatorDescriptor) {
	refs := make([]indicator.Ref, 0, len(requests)*3)
	descriptors := make([]chartIndicatorDescriptor, 0, len(requests)*3)
	add := func(ref indicator.Ref, kind IndicatorKind, component string) {
		refs = append(refs, ref)
		descriptors = append(descriptors, chartIndicatorDescriptor{ref: ref, kind: kind, component: component})
	}
	for _, request := range requests {
		switch request.Kind {
		case IndicatorSMA, IndicatorEMA, IndicatorSTD:
			kind := indicator.SMAKind
			if request.Kind == IndicatorSTD {
				kind = indicator.STDKind
			}
			if request.Kind == IndicatorEMA {
				kind = indicator.EMAKind
			}
			add(indicator.Ref{Kind: kind, Timeframe: query.Timeframe, PriceView: query.View, Field: indicator.Close, Period: request.Period}, request.Kind, "value")
		case IndicatorMACD:
			for _, component := range []struct {
				field indicator.Field
				name  string
			}{{indicator.DIF, "dif"}, {indicator.DEA, "dea"}, {indicator.Histogram, "histogram"}} {
				add(indicator.Ref{Kind: indicator.MACDKind, Timeframe: query.Timeframe, PriceView: query.View, Field: component.field, Fast: request.Fast, Slow: request.Slow, Signal: request.Signal}, request.Kind, component.name)
			}
		case IndicatorKDJ:
			for _, component := range []struct {
				field indicator.Field
				name  string
			}{{indicator.K, "k"}, {indicator.D, "d"}, {indicator.J, "j"}} {
				add(indicator.Ref{Kind: indicator.KDJKind, Timeframe: query.Timeframe, PriceView: query.View, Field: component.field, Period: request.Period}, request.Kind, component.name)
			}
		case IndicatorRETZ:
			for _, component := range []struct {
				field indicator.Field
				name  string
			}{{indicator.Histogram, "histogram"}, {indicator.Smooth, "smooth"}, {indicator.Regime, "regime"}} {
				add(indicator.Ref{Kind: indicator.RETZKind, Timeframe: query.Timeframe, PriceView: query.View, Field: component.field, Period: request.Period, Smooth: request.Smooth, Regime: request.Regime}, request.Kind, component.name)
			}
		}
	}
	return refs, descriptors
}
