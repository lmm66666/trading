package strategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/indicator"
	"trading/internal/market"
)

func TestRegistryReturnsFreshStrategyInstances(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register("daily_b1_buy", "1", strategyFactory(Definition{ID: "daily_b1_buy", Version: "1", PrimaryTimeframe: market.Day})))

	first, err := registry.Resolve("daily_b1_buy", "1", nil)
	require.NoError(t, err)
	second, err := registry.Resolve("daily_b1_buy", "1", nil)
	require.NoError(t, err)

	assert.NotSame(t, first, second)
}

func TestRegistryValidatesParametersBeforeCreatingStrategy(t *testing.T) {
	registry := &Registry{}
	calls := 0
	definition := Definition{
		ID: "daily_b1_buy", Version: "1", PrimaryTimeframe: market.Day,
		Parameters: map[string]ParameterSpec{"hold_bars": {Default: 10, Min: 1, Max: 20, Integer: true}},
	}
	require.NoError(t, registry.Register(definition.ID, definition.Version, func(params map[string]float64) (Strategy, error) {
		calls++
		return &testStrategy{definition: definition}, nil
	}))
	registrationCalls := calls

	_, err := registry.Resolve(definition.ID, definition.Version, map[string]float64{"unknown": 1})
	assert.ErrorIs(t, err, ErrUnknownParameter)
	_, err = registry.Resolve(definition.ID, definition.Version, map[string]float64{"hold_bars": 20.5})
	assert.ErrorIs(t, err, ErrInvalidParameter)
	_, err = registry.Resolve(definition.ID, definition.Version, map[string]float64{"hold_bars": 21})
	assert.ErrorIs(t, err, ErrInvalidParameter)
	assert.Equal(t, registrationCalls, calls)
}

func TestRegistryCopiesDefinitionAndParameters(t *testing.T) {
	registry := &Registry{}
	definition := Definition{
		ID: "daily_b1_buy", Version: "1", PrimaryTimeframe: market.Day,
		Features:   []indicator.Ref{closeRef},
		Parameters: map[string]ParameterSpec{"window": {Default: 2, Min: 1, Max: 3, Integer: true}},
	}
	require.NoError(t, registry.Register(definition.ID, definition.Version, strategyFactory(definition)))

	definition.Features[0] = indicator.Ref{}
	definition.Parameters["window"] = ParameterSpec{Default: 99, Min: 99, Max: 99}
	stored, err := registry.Definition("daily_b1_buy", "1")
	require.NoError(t, err)
	assert.Equal(t, closeRef, stored.Features[0])
	assert.Equal(t, 2.0, stored.Parameters["window"].Default)

	stored.Features[0] = indicator.Ref{}
	stored.Parameters["window"] = ParameterSpec{}
	again, err := registry.Definition("daily_b1_buy", "1")
	require.NoError(t, err)
	assert.Equal(t, closeRef, again.Features[0])
	assert.Equal(t, 2.0, again.Parameters["window"].Default)
}

func TestRegistryRejectsDuplicateAndUnknownStrategies(t *testing.T) {
	registry := &Registry{}
	factory := strategyFactory(Definition{ID: "daily_b1_buy", Version: "1", PrimaryTimeframe: market.Day})
	require.NoError(t, registry.Register("daily_b1_buy", "1", factory))

	assert.ErrorIs(t, registry.Register("daily_b1_buy", "1", factory), ErrDuplicateStrategy)
	_, err := registry.Resolve("missing", "1", nil)
	assert.ErrorIs(t, err, ErrUnknownStrategy)
}

func TestRegistrySuppliesIndependentDefaultParameterMaps(t *testing.T) {
	registry := &Registry{}
	definition := Definition{
		ID: "daily_b1_buy", Version: "1", PrimaryTimeframe: market.Day,
		Parameters: map[string]ParameterSpec{"window": {Default: 2, Min: 1, Max: 3, Integer: true}},
	}
	var received []map[string]float64
	require.NoError(t, registry.Register(definition.ID, definition.Version, func(params map[string]float64) (Strategy, error) {
		if params != nil {
			received = append(received, params)
		}
		return &testStrategy{definition: definition}, nil
	}))

	provided := map[string]float64{"window": 3}
	_, err := registry.Resolve(definition.ID, definition.Version, provided)
	require.NoError(t, err)
	provided["window"] = 1
	_, err = registry.Resolve(definition.ID, definition.Version, nil)
	require.NoError(t, err)

	require.Len(t, received, 2)
	assert.Equal(t, 3.0, received[0]["window"])
	assert.Equal(t, 2.0, received[1]["window"])
	received[0]["window"] = 1
	assert.Equal(t, 2.0, received[1]["window"])
}

func TestRegistryRejectsInvalidDefinitionsAndFactories(t *testing.T) {
	registry := &Registry{}
	assert.ErrorIs(t, registry.Register("x", "1", nil), ErrInvalidDefinition)
	assert.ErrorIs(t, registry.Register("x", "1", strategyFactory(Definition{ID: "other", Version: "1", PrimaryTimeframe: market.Day})), ErrInvalidDefinition)
	assert.ErrorIs(t, registry.Register("x", "1", strategyFactory(Definition{ID: "x", Version: "1", PrimaryTimeframe: market.Day, WarmupBars: -1})), ErrInvalidDefinition)
	assert.ErrorIs(t, registry.Register("x", "1", func(map[string]float64) (Strategy, error) { return nil, assert.AnError }), ErrInvalidDefinition)

	invalidParameter := Definition{
		ID: "x", Version: "1", PrimaryTimeframe: market.Day,
		Parameters: map[string]ParameterSpec{"count": {Default: 1.5, Min: 1, Max: 2, Integer: true}},
	}
	assert.ErrorIs(t, registry.Register("x", "1", strategyFactory(invalidParameter)), ErrInvalidDefinition)
}

func TestRegistryReturnsFactoryFailuresAndRejectsNilResolvedInstances(t *testing.T) {
	registry := &Registry{}
	definition := Definition{ID: "x", Version: "1", PrimaryTimeframe: market.Day}
	calls := 0
	require.NoError(t, registry.Register("x", "1", func(map[string]float64) (Strategy, error) {
		calls++
		if calls > 1 {
			return nil, assert.AnError
		}
		return &testStrategy{definition: definition}, nil
	}))

	_, err := registry.Resolve("x", "1", nil)
	assert.ErrorIs(t, err, assert.AnError)
	_, err = registry.Definition("missing", "1")
	assert.ErrorIs(t, err, ErrUnknownStrategy)
}

type testStrategy struct {
	definition Definition
	onBar      func(Context) (Decision, error)
}

func (s *testStrategy) Definition() Definition { return s.definition }

func (s *testStrategy) OnBar(context Context) (Decision, error) {
	if s.onBar == nil {
		return Decision{Action: Hold}, nil
	}
	return s.onBar(context)
}

func strategyFactory(definition Definition) Factory {
	return func(params map[string]float64) (Strategy, error) {
		return &testStrategy{definition: definition}, nil
	}
}
