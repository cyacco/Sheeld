package guard

import (
	"context"
	"testing"
)

func TestRegexGuard_BlockMode(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		input    string
		wantPass bool
		wantMsg  string
	}{
		{
			name:     "no patterns match",
			patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`}, // SSN pattern
			input:    "no sensitive data here",
			wantPass: true,
			wantMsg:  "no blocked patterns matched",
		},
		{
			name:     "SSN pattern detected",
			patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`},
			input:    "my SSN is 123-45-6789",
			wantPass: false,
			wantMsg:  "input matches blocked patterns",
		},
		{
			name:     "email pattern detected",
			patterns: []string{`[\w.+-]+@[\w-]+\.[\w.]+`},
			input:    "contact me at user@example.com",
			wantPass: false,
			wantMsg:  "input matches blocked patterns",
		},
		{
			name:     "multiple patterns one matches",
			patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`, `[\w.+-]+@[\w-]+\.[\w.]+`},
			input:    "my email is user@example.com",
			wantPass: false,
			wantMsg:  "input matches blocked patterns",
		},
		{
			name:     "multiple patterns none match",
			patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`, `[\w.+-]+@[\w-]+\.[\w.]+`},
			input:    "just a regular message with no PII",
			wantPass: true,
			wantMsg:  "no blocked patterns matched",
		},
		{
			name:     "empty input",
			patterns: []string{`badword`},
			input:    "",
			wantPass: true,
			wantMsg:  "no blocked patterns matched",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewRegexGuard("test-regex", RegexConfig{
				Patterns: tt.patterns,
				Mode:     "block",
			})
			if err != nil {
				t.Fatalf("failed to create guard: %v", err)
			}

			result, err := g.Validate(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Passed != tt.wantPass {
				t.Errorf("got passed=%v, want %v", result.Passed, tt.wantPass)
			}
			if result.Message != tt.wantMsg {
				t.Errorf("got message=%q, want %q", result.Message, tt.wantMsg)
			}
		})
	}
}

func TestRegexGuard_RequireMode(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		input    string
		wantPass bool
	}{
		{
			name:     "all required patterns match",
			patterns: []string{`hello`, `world`},
			input:    "hello beautiful world",
			wantPass: true,
		},
		{
			name:     "one required pattern missing",
			patterns: []string{`hello`, `world`},
			input:    "hello there",
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewRegexGuard("test-regex", RegexConfig{
				Patterns: tt.patterns,
				Mode:     "require",
			})
			if err != nil {
				t.Fatalf("failed to create guard: %v", err)
			}

			result, err := g.Validate(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Passed != tt.wantPass {
				t.Errorf("got passed=%v, want %v", result.Passed, tt.wantPass)
			}
		})
	}
}

func TestRegexGuard_InvalidPattern(t *testing.T) {
	_, err := NewRegexGuard("bad", RegexConfig{
		Patterns: []string{`[invalid`},
		Mode:     "block",
	})
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}
