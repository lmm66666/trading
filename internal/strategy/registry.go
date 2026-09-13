package strategy

import (
	"fmt"
	"reflect"
	"sync"
)

// Factory returns a new strategy instance using fully resolved parameters.
type Factory func(params map[string]float64) (Strategy, error)

type strategyKey struct {
	id      string
	version string
}

type registeredStrategy struct {
	definition Definition
	factory    Factory
}

// Registry is a concurrency-safe catalogue of compiled strategy factories.
type Registry struct {
	mu         sync.RWMutex
	strategies map[strategyKey]registeredStrategy
}

// Register records an immutable strategy definition. The factory is invoked
// once with nil parameters solely to discover its static Definition.
func (r *Registry) Register(id, version string, factory Factory) error {
	if factory == nil {
		return ErrInvalidDefinition
	}
	prototype, err := factory(nil)
	if err != nil || isNilStrategy(prototype) {
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
		}
		return ErrInvalidDefinition
	}
	definition := prototype.Definition()
	if definition.ID != id || definition.Version != version || !validDefinition(definition) {
		return ErrInvalidDefinition
	}
	key := strategyKey{id: id, version: version}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.strategies == nil {
		r.strategies = make(map[strategyKey]registeredStrategy)
	}
	if _, exists := r.strategies[key]; exists {
		return ErrDuplicateStrategy
	}
	r.strategies[key] = registeredStrategy{definition: cloneDefinition(definition), factory: factory}
	return nil
}

// Definition returns an independent copy of the registered static contract.
func (r *Registry) Definition(id, version string) (Definition, error) {
	registered, ok := r.lookup(id, version)
	if !ok {
		return Definition{}, ErrUnknownStrategy
	}
	return cloneDefinition(registered.definition), nil
}

// Resolve validates and defaults parameters before constructing a new instance.
func (r *Registry) Resolve(id, version string, params map[string]float64) (Strategy, error) {
	registered, ok := r.lookup(id, version)
	if !ok {
		return nil, ErrUnknownStrategy
	}
	resolved, err := resolveParameters(registered.definition.Parameters, params)
	if err != nil {
		return nil, err
	}
	strategy, err := registered.factory(resolved)
	if err != nil {
		return nil, err
	}
	if isNilStrategy(strategy) {
		return nil, ErrInvalidDefinition
	}
	definition := strategy.Definition()
	if definition.ID != registered.definition.ID || definition.Version != registered.definition.Version {
		return nil, ErrInvalidDefinition
	}
	return strategy, nil
}

func (r *Registry) lookup(id, version string) (registeredStrategy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	registered, ok := r.strategies[strategyKey{id: id, version: version}]
	if !ok {
		return registeredStrategy{}, false
	}
	registered.definition = cloneDefinition(registered.definition)
	return registered, true
}

func resolveParameters(specifications map[string]ParameterSpec, supplied map[string]float64) (map[string]float64, error) {
	resolved := make(map[string]float64, len(specifications))
	for name, specification := range specifications {
		resolved[name] = specification.Default
	}
	for name, value := range supplied {
		specification, exists := specifications[name]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrUnknownParameter, name)
		}
		if !validParameter(specification, value) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidParameter, name)
		}
		resolved[name] = value
	}
	return resolved, nil
}

func isNilStrategy(strategy Strategy) bool {
	if strategy == nil {
		return true
	}
	value := reflect.ValueOf(strategy)
	return value.Kind() == reflect.Ptr && value.IsNil()
}
