package mysql

import (
	"fmt"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{port.ErrInvalidPortValue}, args...)...)
}

// MarketBatchDigest 保留存储调用方兼容入口；规范化契约由 port 统一维护。
func MarketBatchDigest(batch port.MarketWriteBatch) (string, error) {
	return port.MarketBatchDigest(batch)
}
func canonicalBatch(batch port.MarketWriteBatch) (port.MarketWriteBatch, string, error) {
	return port.CanonicalMarketBatch(batch)
}
func storedTime(t time.Time) error                  { return port.ValidateMarketTime(t) }
func validateStoredBar(b market.Bar) error          { return port.ValidateMarketBar(b) }
func validateAction(a market.CorporateAction) error { return port.ValidateCorporateAction(a) }

func instrumentModel(id market.InstrumentID, source string) (InstrumentModel, error) {
	if err := id.Validate(); err != nil {
		return InstrumentModel{}, invalid("invalid instrument: %v", err)
	}
	delivery, hasDelivery := id.DeliveryMonth()
	var deliveryMonth *time.Time
	if hasDelivery {
		deliveryMonth = &delivery
	}
	return InstrumentModel{
		Exchange:       string(id.Exchange),
		Code:           id.Code,
		AssetClass:     string(id.AssetClass()),
		InstrumentKind: string(id.Kind()),
		ProductCode:    id.Product(),
		DeliveryMonth:  deliveryMonth,
		Source:         source,
	}, nil
}

func timeframeName(tf market.Timeframe) string {
	switch tf {
	case market.Day:
		return "DAY"
	case market.Week:
		return "WEEK"
	case market.Month:
		return "MONTH"
	}
	return ""
}
func parseTimeframe(tf string) (market.Timeframe, error) {
	switch tf {
	case "DAY":
		return market.Day, nil
	case "WEEK":
		return market.Week, nil
	case "MONTH":
		return market.Month, nil
	}
	return 0, invalid("unknown stored timeframe %q", tf)
}
func priceInt(p *market.Price) *int64 {
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}
func intPrice(p *int64) *market.Price {
	if p == nil {
		return nil
	}
	v := market.Price(*p)
	return &v
}

func barModel(id uint64, b market.Bar, revision uint32, version uint64) MarketBarModel {
	status := "TRADABLE"
	if b.Trading == market.Suspended {
		status = "SUSPENDED"
	}
	return MarketBarModel{InstrumentID: id, Timeframe: timeframeName(b.Timeframe), OpenTime: b.OpenTime, CloseTime: b.CloseTime, Revision: revision, ValidFromVersion: version, Open: int64(b.Open), High: int64(b.High), Low: int64(b.Low), Close: int64(b.Close), Volume: b.Volume, Amount: int64(b.Amount), TradingStatus: status, LimitUp: priceInt(b.LimitUp), LimitDown: priceInt(b.LimitDown)}
}

func (row MarketBarModel) bar(id market.InstrumentID, version market.DataVersion) (market.Bar, error) {
	tf, err := parseTimeframe(row.Timeframe)
	if err != nil {
		return market.Bar{}, err
	}
	status := market.Tradable
	switch row.TradingStatus {
	case "TRADABLE":
	case "SUSPENDED":
		status = market.Suspended
	default:
		return market.Bar{}, invalid("unknown stored trading status")
	}
	b := market.Bar{Instrument: id, Timeframe: tf, OpenTime: row.OpenTime.UTC(), CloseTime: row.CloseTime.UTC(), Version: version, Open: market.Price(row.Open), High: market.Price(row.High), Low: market.Price(row.Low), Close: market.Price(row.Close), Volume: row.Volume, Amount: market.Money(row.Amount), Trading: status, LimitUp: intPrice(row.LimitUp), LimitDown: intPrice(row.LimitDown)}
	return b, validateStoredBar(b)
}

func actionName(kind market.CorporateActionKind) string {
	switch kind {
	case market.CashDividend:
		return "CASH_DIVIDEND"
	case market.ShareDistribution:
		return "SHARE_DISTRIBUTION"
	case market.RightsIssue:
		return "RIGHTS_ISSUE"
	}
	return ""
}
func actionModel(id uint64, a market.CorporateAction, v uint64) CorporateActionModel {
	return CorporateActionModel{InstrumentID: id, SourceEventID: a.ID, ExDate: a.ExDate, Kind: actionName(a.Kind), CashPerShare: int64(a.CashPerShare), ShareNumerator: a.ShareNumerator, ShareDenominator: a.ShareDenominator, ValidFromVersion: v}
}
func (row CorporateActionModel) action(id market.InstrumentID, v market.DataVersion) (market.CorporateAction, error) {
	var kind market.CorporateActionKind
	switch row.Kind {
	case "CASH_DIVIDEND":
		kind = market.CashDividend
	case "SHARE_DISTRIBUTION":
		kind = market.ShareDistribution
	case "RIGHTS_ISSUE":
		kind = market.RightsIssue
	}
	a := market.CorporateAction{ID: row.SourceEventID, Instrument: id, ExDate: row.ExDate.UTC(), Kind: kind, CashPerShare: market.Money(row.CashPerShare), ShareNumerator: row.ShareNumerator, ShareDenominator: row.ShareDenominator, Version: v}
	return a, validateAction(a)
}
