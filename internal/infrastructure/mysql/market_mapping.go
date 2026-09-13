package mysql

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
	"trading/internal/market"
	"trading/internal/port"
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{port.ErrInvalidPortValue}, args...)...)
}

// MarketBatchDigest computes the SHA-256 of the canonical incremental batch.
// Input version metadata is excluded: the writer assigns publication versions.
func MarketBatchDigest(batch port.MarketWriteBatch) (string, error) {
	batch.Digest = ""
	_, digest, err := canonicalBatch(batch)
	return digest, err
}

func canonicalBatch(input port.MarketWriteBatch) (port.MarketWriteBatch, string, error) {
	b := input
	b.Digest = "canonicalizing"
	if err := b.Validate(); err != nil {
		return b, "", err
	}
	b.Bars = make(map[market.Timeframe][]market.Bar, len(input.Bars))
	for tf, bars := range input.Bars {
		copyBars := make([]market.Bar, len(bars))
		for i, bar := range bars {
			bar.Version = 0
			if err := validateStoredBar(bar); err != nil {
				return b, "", err
			}
			copyBars[i] = bar
		}
		dataset, err := market.NewDataset(b.Instrument, tf, 0, copyBars)
		if err != nil {
			return b, "", fmt.Errorf("canonical bars: %w", err)
		}
		if dataset.Len() > 0 {
			b.Bars[tf] = dataset.Bars()
		}
	}
	b.Factors = append([]market.AdjustmentFactor{}, input.Factors...)
	sort.Slice(b.Factors, func(i, j int) bool { return b.Factors[i].EffectiveTime.Before(b.Factors[j].EffectiveTime) })
	for i := range b.Factors {
		b.Factors[i].Version = 0
		if err := storedTime(b.Factors[i].EffectiveTime); err != nil {
			return b, "", err
		}
		if i > 0 && b.Factors[i].EffectiveTime.Equal(b.Factors[i-1].EffectiveTime) {
			return b, "", invalid("duplicate factor effective time")
		}
	}
	b.Actions = append([]market.CorporateAction{}, input.Actions...)
	sort.Slice(b.Actions, func(i, j int) bool { return b.Actions[i].ID < b.Actions[j].ID })
	for i := range b.Actions {
		b.Actions[i].Version = 0
		if err := validateAction(b.Actions[i]); err != nil {
			return b, "", err
		}
		if i > 0 && b.Actions[i].ID == b.Actions[i-1].ID {
			return b, "", invalid("duplicate action ID")
		}
	}
	if len(b.Bars) == 0 && len(b.Factors) == 0 && len(b.Actions) == 0 {
		return b, "", invalid("batch contains no observations")
	}
	b.Digest = ""
	encoded, err := json.Marshal(b)
	if err != nil {
		return b, "", fmt.Errorf("encode canonical batch: %w", err)
	}
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	if input.Digest != "" && input.Digest != digest {
		return b, "", invalid("batch digest does not match canonical SHA-256")
	}
	b.Digest = digest
	return b, digest, nil
}

func storedTime(t time.Time) error {
	if t.IsZero() || t.Location() != time.UTC || t.Year() < 1000 || t.Year() > 9999 || t.Nanosecond()%1000 != 0 {
		return invalid("timestamp must be UTC and exactly representable by MySQL datetime(6)")
	}
	return nil
}

func validateStoredBar(b market.Bar) error {
	if err := storedTime(b.OpenTime); err != nil {
		return err
	}
	if err := storedTime(b.CloseTime); err != nil {
		return err
	}
	if b.CloseTime.Before(b.OpenTime) {
		return invalid("bar closes before it opens")
	}
	if b.Trading != market.Tradable && b.Trading != market.Suspended {
		return invalid("unknown trading status")
	}
	if b.Amount < 0 {
		return invalid("negative bar amount")
	}
	if (b.LimitUp != nil && *b.LimitUp <= 0) || (b.LimitDown != nil && *b.LimitDown <= 0) || (b.LimitUp != nil && b.LimitDown != nil && *b.LimitDown > *b.LimitUp) {
		return invalid("invalid price limits")
	}
	return nil
}

func validateAction(a market.CorporateAction) error {
	if err := port.ValidateIdentity(a.ID, "source event ID", port.MaxSourceEventIDBytes, false); err != nil {
		return err
	}
	if err := a.Instrument.Validate(); err != nil {
		return err
	}
	if err := storedTime(a.ExDate); err != nil {
		return err
	}
	if a.CashPerShare < 0 || a.ShareNumerator < 0 || a.ShareDenominator < 0 {
		return invalid("negative corporate action value")
	}
	switch a.Kind {
	case market.CashDividend:
		if a.ShareNumerator != 0 || a.ShareDenominator != 0 {
			return invalid("cash dividend has share ratio")
		}
	case market.ShareDistribution:
		if a.ShareNumerator <= 0 || a.ShareDenominator <= 0 {
			return invalid("share distribution requires a positive ratio")
		}
	case market.RightsIssue: // Keep unsupported actions explicit for the engine's fail-fast path.
	default:
		return invalid("unknown corporate action kind")
	}
	return nil
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
