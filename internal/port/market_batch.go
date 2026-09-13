package port

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
	"trading/internal/market"
)

// MarketBatchDigest computes the SHA-256 of the canonical incremental batch.
// Input version metadata is excluded: the writer assigns publication versions.
func MarketBatchDigest(batch MarketWriteBatch) (string, error) {
	batch.Digest = ""
	_, digest, err := CanonicalMarketBatch(batch)
	return digest, err
}

func CanonicalMarketBatch(input MarketWriteBatch) (MarketWriteBatch, string, error) {
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
			if err := ValidateMarketBar(bar); err != nil {
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
		if err := ValidateMarketTime(b.Factors[i].EffectiveTime); err != nil {
			return b, "", err
		}
		if i > 0 && b.Factors[i].EffectiveTime.Equal(b.Factors[i-1].EffectiveTime) {
			return b, "", invalidPortValue("duplicate factor effective time")
		}
	}
	b.Actions = append([]market.CorporateAction{}, input.Actions...)
	sort.Slice(b.Actions, func(i, j int) bool { return b.Actions[i].ID < b.Actions[j].ID })
	for i := range b.Actions {
		b.Actions[i].Version = 0
		if err := ValidateCorporateAction(b.Actions[i]); err != nil {
			return b, "", err
		}
		if i > 0 && b.Actions[i].ID == b.Actions[i-1].ID {
			return b, "", invalidPortValue("duplicate action ID")
		}
	}
	if len(b.Bars) == 0 && len(b.Factors) == 0 && len(b.Actions) == 0 {
		return b, "", invalidPortValue("batch contains no observations")
	}
	b.Digest = ""
	encoded, err := json.Marshal(b)
	if err != nil {
		return b, "", fmt.Errorf("encode canonical batch: %w", err)
	}
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	if input.Digest != "" && input.Digest != digest {
		return b, "", invalidPortValue("batch digest does not match canonical SHA-256")
	}
	b.Digest = digest
	return b, digest, nil
}

func ValidateMarketTime(t time.Time) error {
	if t.IsZero() || t.Location() != time.UTC || t.Year() < 1000 || t.Year() > 9999 || t.Nanosecond()%1000 != 0 {
		return invalidPortValue("timestamp must be UTC and exactly representable by MySQL datetime(6)")
	}
	return nil
}

func ValidateMarketBar(b market.Bar) error {
	if err := ValidateMarketTime(b.OpenTime); err != nil {
		return err
	}
	if err := ValidateMarketTime(b.CloseTime); err != nil {
		return err
	}
	if b.CloseTime.Before(b.OpenTime) {
		return invalidPortValue("bar closes before it opens")
	}
	if b.Trading != market.Tradable && b.Trading != market.Suspended {
		return invalidPortValue("unknown trading status")
	}
	if b.Amount < 0 {
		return invalidPortValue("negative bar amount")
	}
	if (b.LimitUp != nil && *b.LimitUp <= 0) || (b.LimitDown != nil && *b.LimitDown <= 0) || (b.LimitUp != nil && b.LimitDown != nil && *b.LimitDown > *b.LimitUp) {
		return invalidPortValue("invalid price limits")
	}
	return nil
}

func ValidateCorporateAction(a market.CorporateAction) error {
	if err := ValidateIdentity(a.ID, "source event ID", MaxSourceEventIDBytes, false); err != nil {
		return err
	}
	if err := a.Instrument.Validate(); err != nil {
		return err
	}
	if err := ValidateMarketTime(a.ExDate); err != nil {
		return err
	}
	if a.CashPerShare < 0 || a.ShareNumerator < 0 || a.ShareDenominator < 0 {
		return invalidPortValue("negative corporate action value")
	}
	switch a.Kind {
	case market.CashDividend:
		if a.ShareNumerator != 0 || a.ShareDenominator != 0 {
			return invalidPortValue("cash dividend has share ratio")
		}
	case market.ShareDistribution:
		if a.ShareNumerator <= 0 || a.ShareDenominator <= 0 {
			return invalidPortValue("share distribution requires a positive ratio")
		}
	case market.RightsIssue: // Keep unsupported actions explicit for the engine's fail-fast path.
	default:
		return invalidPortValue("unknown corporate action kind")
	}
	return nil
}
