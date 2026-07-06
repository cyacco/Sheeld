package transform

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
)

// Factory creates a Transformer instance from a name and JSON configuration.
type Factory func(name string, config json.RawMessage) (Transformer, error)

// Registry maps transformer type strings to their factory functions.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates a transformer registry with built-in types registered.
func NewRegistry() *Registry {
	r := &Registry{factories: make(map[string]Factory)}

	// Register built-in transformer types
	r.Register("regex_replace", regexReplaceFactory)
	r.Register("webhook", webhookFactory)
	r.Register("presidio", presidioFactory)

	return r
}

// --- Built-in factories ---

func regexReplaceFactory(name string, config json.RawMessage) (Transformer, error) {
	var cfg RegexReplaceConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid regex_replace config: %w", err)
	}
	if len(cfg.Rules) == 0 {
		return nil, fmt.Errorf("regex_replace: at least one rule is required")
	}
	for _, rule := range cfg.Rules {
		if rule.Pattern == "" {
			return nil, fmt.Errorf("regex_replace: rule pattern is required")
		}
	}
	return NewRegexReplaceTransformer(name, cfg)
}

func webhookFactory(name string, config json.RawMessage) (Transformer, error) {
	var cfg WebhookConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid webhook config: %w", err)
	}
	if err := validateHTTPURL(cfg.URL, "webhook: url"); err != nil {
		return nil, err
	}
	return NewWebhookTransformer(name, cfg), nil
}

func presidioFactory(name string, config json.RawMessage) (Transformer, error) {
	var cfg PresidioConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("invalid presidio config: %w", err)
	}
	if err := validateHTTPURL(cfg.AnalyzerURL, "presidio: analyzer_url"); err != nil {
		return nil, err
	}
	if err := validateHTTPURL(cfg.AnonymizerURL, "presidio: anonymizer_url"); err != nil {
		return nil, err
	}
	return NewPresidioTransformer(name, cfg), nil
}

func validateHTTPURL(raw, field string) error {
	if raw == "" {
		return fmt.Errorf("%s is required", field)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s must be a valid http(s) URL", field)
	}
	return nil
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
