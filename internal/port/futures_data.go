package port

import (
	"context"

	"trading/internal/market"
)

type FuturesDataWriter interface {
	PublishContract(context.Context, FuturesContractBatch) (market.DataVersion, error)
	PublishContinuous(context.Context, FuturesContinuousBatch) (market.DataVersion, error)
}

type FuturesContractBatch struct {
	Source     string
	Instrument market.InstrumentID
	Daily      []market.FuturesDaily
	Digest     string
}

type FuturesContinuousBatch struct {
	Source         string
	Instrument     market.InstrumentID
	Bars           []market.Bar
	Mappings       []market.MainMapping
	DependsThrough market.DataVersion
	Digest         string
}
