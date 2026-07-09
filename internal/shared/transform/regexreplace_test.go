package transform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

func TestRegexReplaceTransform(t *testing.T) {
	tr, err := NewRegexReplaceTransformer("mask", RegexReplaceConfig{
		Rules: []RegexReplaceRule{
			{Pattern: `\b\d{3}-\d{2}-\d{4}\b`, Replace: "[SSN]"},
			{Pattern: `(?i)password:\s*(\S+)`, Replace: "password: [REDACTED]"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	in := []llm.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "my ssn is 123-45-6789 and Password: hunter2"},
	}
	out, err := tr.Transform(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out[1].Content, "my ssn is [SSN] and password: [REDACTED]"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if out[0].Content != "You are helpful." {
		t.Errorf("system message changed: %q", out[0].Content)
	}
	if in[1].Content != "my ssn is 123-45-6789 and Password: hunter2" {
		t.Error("input slice was mutated")
	}
}

func TestRegexReplaceGroupReferences(t *testing.T) {
	tr, err := NewRegexReplaceTransformer("swap", RegexReplaceConfig{
		Rules: []RegexReplaceRule{{Pattern: `(\w+)@(\w+)\.com`, Replace: "$1@[DOMAIN]"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tr.Transform(context.Background(), []llm.Message{{Role: "user", Content: "mail bob@acme.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out[0].Content, "mail bob@[DOMAIN]"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestRegexReplaceFactoryValidation(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{"no rules", `{"rules":[]}`},
		{"empty pattern", `{"rules":[{"pattern":"","replace":"x"}]}`},
		{"invalid pattern", `{"rules":[{"pattern":"[","replace":"x"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := regexReplaceFactory("t", json.RawMessage(tt.config)); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
	if _, err := regexReplaceFactory("t", json.RawMessage(`{"rules":[{"pattern":"a","replace":"b"}]}`)); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}
