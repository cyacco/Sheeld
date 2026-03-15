package guard

import (
	"encoding/json"
	"testing"
)

func TestRegistry_BuiltInTypes(t *testing.T) {
	r := NewRegistry()

	types := r.Types()
	if len(types) < 2 {
		t.Fatalf("expected at least 2 built-in types, got %d", len(types))
	}

	// Verify blocklist and regex are registered
	typeSet := make(map[string]bool)
	for _, typ := range types {
		typeSet[typ] = true
	}

	if !typeSet["blocklist"] {
		t.Error("expected 'blocklist' to be registered")
	}
	if !typeSet["regex"] {
		t.Error("expected 'regex' to be registered")
	}
}

func TestRegistry_CreateBlocklist(t *testing.T) {
	r := NewRegistry()

	config := json.RawMessage(`{"words": ["bad", "evil"]}`)
	g, err := r.Create("blocklist", "test-blocklist", config)
	if err != nil {
		t.Fatalf("failed to create blocklist guard: %v", err)
	}

	if g.Type() != "blocklist" {
		t.Errorf("got type=%q, want %q", g.Type(), "blocklist")
	}
	if g.Name() != "test-blocklist" {
		t.Errorf("got name=%q, want %q", g.Name(), "test-blocklist")
	}
}

func TestRegistry_CreateRegex(t *testing.T) {
	r := NewRegistry()

	config := json.RawMessage(`{"patterns": ["\\bfoo\\b"], "mode": "block"}`)
	g, err := r.Create("regex", "test-regex", config)
	if err != nil {
		t.Fatalf("failed to create regex guard: %v", err)
	}

	if g.Type() != "regex" {
		t.Errorf("got type=%q, want %q", g.Type(), "regex")
	}
}

func TestRegistry_UnknownType(t *testing.T) {
	r := NewRegistry()

	_, err := r.Create("nonexistent", "test", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for unknown guard type")
	}
}

func TestRegistry_InvalidConfig(t *testing.T) {
	r := NewRegistry()

	_, err := r.Create("blocklist", "test", json.RawMessage(`not json`))
	if err == nil {
		t.Error("expected error for invalid config JSON")
	}
}

func TestRegistry_CustomType(t *testing.T) {
	r := NewRegistry()

	// Register a custom factory
	r.Register("custom", func(name string, config json.RawMessage) (Guard, error) {
		return NewBlocklistGuard(name, BlocklistConfig{Words: []string{"custom"}}), nil
	})

	g, err := r.Create("custom", "my-custom", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("failed to create custom guard: %v", err)
	}

	if g.Type() != "blocklist" {
		t.Errorf("got type=%q, want %q", g.Type(), "blocklist")
	}
	if g.Name() != "my-custom" {
		t.Errorf("got name=%q, want %q", g.Name(), "my-custom")
	}
}
