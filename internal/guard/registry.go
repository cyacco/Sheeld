package guard

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Factory creates a Guard instance from a name and JSON configuration.
type Factory func(name string, config json.RawMessage) (Guard, error)

// Registry maps guard type strings to their factory functions.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates a new guard registry with built-in guard types registered.
func NewRegistry() *Registry {
	r := &Registry{
		factories: make(map[string]Factory),
	}

	// Register built-in guard types
	r.Register("blocklist", blocklistFactory)
	r.Register("regex", regexFactory)
	r.Register("openai_moderation", openAIModerationFactory)
	r.Register("guardrails_ai", guardrailsAIFactory)

	return r
}

// Register adds a guard type factory to the registry.
func (r *Registry) Register(guardType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[guardType] = factory
}

// Create instantiates a guard from its type, name, and config.
func (r *Registry) Create(guardType string, name string, config json.RawMessage) (Guard, error) {
	r.mu.RLock()
	factory, ok := r.factories[guardType]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown guard type: %q", guardType)
	}

	return factory(name, config)
}

// Types returns all registered guard type names.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// --- Built-in factories ---

func blocklistFactory(name string, config json.RawMessage) (Guard, error) {
	var cfg BlocklistConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid blocklist config: %w", err)
	}
	return NewBlocklistGuard(name, cfg), nil
}

func regexFactory(name string, config json.RawMessage) (Guard, error) {
	var cfg RegexConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid regex config: %w", err)
	}
	return NewRegexGuard(name, cfg)
}

func openAIModerationFactory(name string, config json.RawMessage) (Guard, error) {
	var cfg OpenAIModerationConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid openai_moderation config: %w", err)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai_moderation: api_key is required")
	}
	return NewOpenAIModerationGuard(name, cfg), nil
}

func guardrailsAIFactory(name string, config json.RawMessage) (Guard, error) {
	var cfg GuardrailsAIConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid guardrails_ai config: %w", err)
	}
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("guardrails_ai: server_url is required")
	}
	if cfg.GuardName == "" {
		return nil, fmt.Errorf("guardrails_ai: guard_name is required")
	}
	return NewGuardrailsAIGuard(name, cfg), nil
}
