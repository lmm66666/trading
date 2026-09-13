package application

import (
	"context"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

const MaxPriceBars = 5000

type PriceQuery struct {
	Instrument market.InstrumentID
	Timeframe  market.Timeframe
	View       market.PriceView
	From, To   time.Time
	Version    market.DataVersion
	Limit      int
}
type PriceBar struct {
	OpenTime  time.Time            `json:"open_time"`
	CloseTime time.Time            `json:"close_time"`
	Open      float64              `json:"open"`
	High      float64              `json:"high"`
	Low       float64              `json:"low"`
	Close     float64              `json:"close"`
	Volume    int64                `json:"volume"`
	Amount    float64              `json:"amount"`
	Trading   market.TradingStatus `json:"trading_status"`
}
type PriceResult struct {
	Instrument  market.InstrumentID `json:"instrument"`
	Timeframe   market.Timeframe    `json:"timeframe"`
	View        market.PriceView    `json:"view"`
	DataVersion market.DataVersion  `json:"data_version"`
	Bars        []PriceBar          `json:"bars"`
}
type MarketQueryService struct{ data port.MarketData }

func NewMarketQueryService(data port.MarketData) *MarketQueryService {
	return &MarketQueryService{data: data}
}

// Prices 返回窗口内最近 Limit 根，按收盘时间升序；time.Time 在 API JSON 边界输出 UTC RFC3339。
func (s *MarketQueryService) Prices(ctx context.Context, q PriceQuery) (PriceResult, error) {
	result := PriceResult{}
	if err := q.Instrument.Validate(); err != nil {
		return result, invalidRequest("invalid instrument")
	}
	if q.Timeframe != market.Day && q.Timeframe != market.Week {
		return result, invalidRequest("unsupported timeframe")
	}
	if q.View != market.Raw && q.View != market.ForwardAdjusted {
		return result, invalidRequest("invalid price view")
	}
	if q.Limit < 0 || q.Limit > MaxPriceBars {
		return result, invalidRequest("price limit exceeds bounds")
	}
	if q.Limit == 0 {
		q.Limit = MaxPriceBars
	}
	if q.To.IsZero() {
		q.To = time.Now().UTC()
	}
	if q.From.IsZero() {
		q.From = q.To.AddDate(-port.MaxBacktestRangeYears, 0, 0)
	}
	if err := validateUTCDate(q.From, "from"); err != nil {
		return result, err
	}
	if err := validateUTCDate(q.To, "to"); err != nil {
		return result, err
	}
	if q.To.Before(q.From) || q.To.After(q.From.AddDate(port.MaxBacktestRangeYears, 0, 0)) {
		return result, invalidRequest("invalid price window")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s.data == nil {
		return result, invalidRequest("market data is required")
	}
	var err error
	if q.Version == 0 {
		q.Version, err = s.data.LatestCompleteVersion(ctx)
		if err != nil {
			return result, err
		}
	}
	if q.Version == 0 {
		return result, ErrIncompleteMarketData
	}
	ds, factors, _, err := s.data.Dataset(ctx, q.Instrument, q.Timeframe, q.From, q.To, q.Version)
	if err != nil {
		return result, err
	}
	if ds.Instrument() != q.Instrument || ds.Timeframe() != q.Timeframe || ds.Version() != q.Version {
		return result, ErrIncompleteMarketData
	}
	if q.View == market.ForwardAdjusted {
		for _, f := range factors {
			if f.Version != q.Version {
				return result, ErrIncompleteMarketData
			}
		}
		factors, err = normalizeFactors(factors)
		if err != nil {
			return result, err
		}
	}
	result = PriceResult{Instrument: q.Instrument, Timeframe: q.Timeframe, View: q.View, DataVersion: q.Version, Bars: make([]PriceBar, 0)}
	start := ds.Len() - q.Limit
	if start < 0 {
		start = 0
	}
	factorIndex := 0
	for i := start; i < ds.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return PriceResult{}, err
		}
		b := ds.Bar(i)
		if b.CloseTime.Before(q.From) || b.CloseTime.After(q.To) {
			return PriceResult{}, ErrIncompleteMarketData
		}
		multiplier := 1.0 / float64(market.ValueScale)
		if q.View == market.ForwardAdjusted {
			f, ok := factorAt(factors, &factorIndex, b.CloseTime)
			if !ok {
				return PriceResult{}, ErrIncompleteMarketData
			}
			multiplier *= float64(f.Numerator) / float64(f.Denominator)
		}
		result.Bars = append(result.Bars, PriceBar{OpenTime: b.OpenTime.UTC(), CloseTime: b.CloseTime.UTC(), Open: float64(b.Open) * multiplier, High: float64(b.High) * multiplier, Low: float64(b.Low) * multiplier, Close: float64(b.Close) * multiplier, Volume: b.Volume, Amount: float64(b.Amount) / float64(market.ValueScale), Trading: b.Trading})
	}
	return result, nil
}
