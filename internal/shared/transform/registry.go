package transform

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Factory creates a Transformer instance from a name and JSON configuration.
type Factory func(name string, config json.RawMessage) (Transformer, error)

// Registry maps transformer type strings to their factory functions.
// v1 ships no built-in types; callers (and tests) register types via
// Register.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates an empty transformer registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register adds a transformer type factory to the registry.
func (r *Registry) Register(transformerType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[transformerType] = factory
}

// Has reports whether a transformer type is registered.
func (r *Registry) Has(transformerType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[transformerType]
	return ok
}

// Create instantiates a transformer from its type, name, and config.
func (r *Registry) Create(transformerType string, name string, config json.RawMessage) (Transformer, error) {
	r.mu.RLock()
	factory, ok := r.factories[transformerType]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown transformer type: %q", transformerType)
	}
	return factory(name, config)
}

// Types returns all registered transformer type names.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}
