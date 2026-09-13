package port

import (
	"context"

	"trading/internal/market"
)

type MarketDataWriter interface {
	Publish(ctx context.Context, batch MarketWriteBatch) (market.DataVersion, error)
}

// MarketWriteBatch is an immutable publication candidate. Bars, Factors and
// Actions remain owned by the caller; writers must copy before retaining them.
type MarketWriteBatch struct {
	Source     string                            `json:"source"`
	Instrument market.InstrumentID               `json:"instrument"`
	Bars       map[market.Timeframe][]market.Bar `json:"bars,omitempty"`
	Factors    []market.AdjustmentFactor         `json:"factors,omitempty"`
	Actions    []market.CorporateAction          `json:"actions,omitempty"`
	Digest     string                            `json:"digest"`
}

func (batch MarketWriteBatch) Validate() error {
	if err := ValidateIdentity(batch.Source, "source", MaxMarketSourceBytes, false); err != nil {
		return err
	}
	if err := ValidateIdentity(batch.Digest, "digest", MaxHashBytes, false); err != nil {
		return err
	}
	if err := validateInstrument(batch.Instrument, "instrument"); err != nil {
		return err
	}
	if len(batch.Bars) == 0 && len(batch.Factors) == 0 && len(batch.Actions) == 0 {
		return invalidPortValue("batch is empty")
	}
	for timeframe, bars := range batch.Bars {
		if !timeframe.Valid() {
			return invalidPortValue("bar timeframe is invalid")
		}
		for _, bar := range bars {
			if bar.Instrument != batch.Instrument || bar.Timeframe != timeframe {
				return invalidPortValue("bar does not match batch instrument or timeframe")
			}
		}
	}
	for _, factor := range batch.Factors {
		if err := validateUTCTime(factor.EffectiveTime, "factor effective time", false); err != nil {
			return err
		}
		if factor.Numerator <= 0 || factor.Denominator <= 0 {
			return invalidPortValue("adjustment factor must be positive")
		}
	}
	for _, action := range batch.Actions {
		if err := ValidateIdentity(action.ID, "source event ID", MaxSourceEventIDBytes, false); err != nil {
			return err
		}
		if action.Instrument != batch.Instrument {
			return invalidPortValue("corporate action is invalid")
		}
		if err := validateUTCTime(action.ExDate, "corporate action ex date", false); err != nil {
			return err
		}
	}
	return nil
}
