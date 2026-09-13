package port

import (
	"context"
	"sort"
	"time"

	"trading/internal/market"
)

type FuturesSource interface {
	FetchDaily(ctx context.Context, exchange market.Exchange, tradeDate time.Time, products []string) (FuturesPartition, error)
}

type FuturesPartition struct {
	Exchange        market.Exchange
	TradeDate       time.Time
	Rows            []market.FuturesDaily
	Provider        string
	ProviderVersion string
	EmptyReason     string
}

func CanonicalFuturesPartition(input FuturesPartition) (FuturesPartition, error) {
	if !validFuturesExchange(input.Exchange) || !isUTCDate(input.TradeDate) {
		return FuturesPartition{}, invalidPortValue("invalid futures partition identity")
	}
	if err := ValidateIdentity(input.Provider, "provider", MaxMarketSourceBytes, false); err != nil {
		return FuturesPartition{}, err
	}
	if err := ValidateIdentity(input.ProviderVersion, "provider version", MaxStrategyVersionBytes, false); err != nil {
		return FuturesPartition{}, err
	}
	if len(input.Rows) == 0 && input.EmptyReason == "" {
		return FuturesPartition{}, invalidPortValue("empty futures partition requires a reason")
	}
	output := input
	output.Rows = append([]market.FuturesDaily(nil), input.Rows...)
	sort.Slice(output.Rows, func(i, j int) bool { return output.Rows[i].Instrument.String() < output.Rows[j].Instrument.String() })
	for index, row := range output.Rows {
		if err := row.Validate(); err != nil {
			return FuturesPartition{}, invalidPortValue("invalid futures row: %v", err)
		}
		if row.Instrument.Exchange != input.Exchange || row.TradeDate != input.TradeDate || row.ProviderVersion != input.ProviderVersion {
			return FuturesPartition{}, invalidPortValue("futures row does not match partition")
		}
		if index > 0 && row.Instrument == output.Rows[index-1].Instrument {
			return FuturesPartition{}, invalidPortValue("duplicate futures contract")
		}
	}
	return output, nil
}

func validFuturesExchange(exchange market.Exchange) bool {
	switch exchange {
	case market.SHFE, market.INE, market.DCE, market.CZCE:
		return true
	default:
		return false
	}
}

func isUTCDate(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}
