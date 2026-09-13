package strategy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"trading/internal/indicator"
	"trading/internal/market"
	"trading/internal/strategy"
)

func TestValidateDefinitionRejectsCompleteInvalidContractSet(t *testing.T) {
	base := strategy.Definition{ID: "test", Version: "1", PrimaryTimeframe: market.Day}
	weekFeature := indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Week, PriceView: market.Raw, Field: indicator.Close, Period: 1}
	dayFeature := indicator.Ref{Kind: indicator.SMAKind, Timeframe: market.Day, PriceView: market.Raw, Field: indicator.Close, Period: 1}
	cases := []strategy.Definition{
		{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Auxiliary: []market.Timeframe{market.Day}},
		{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Auxiliary: []market.Timeframe{market.Week, market.Week}},
		{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{weekFeature}},
		{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Features: []indicator.Ref{dayFeature, dayFeature}},
		{ID: "test", Version: "1", PrimaryTimeframe: market.Day, Parameters: map[string]strategy.ParameterSpec{"bad": {Default: 2, Min: 0, Max: 1}}},
	}
	assert.NoError(t, strategy.ValidateDefinition(base))
	for _, definition := range cases {
		assert.ErrorIs(t, strategy.ValidateDefinition(definition), strategy.ErrInvalidDefinition)
	}
}
