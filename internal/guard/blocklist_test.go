package guard

import (
	"context"
	"testing"
)

func TestBlocklistGuard_BlockMode(t *testing.T) {
	tests := []struct {
		name        string
		words       []string
		input       string
		wantPass    bool
		wantMsg     string
		wantMatched []string
	}{
		{
			name:     "no blocked words found",
			words:    []string{"bad", "evil"},
			input:    "this is a perfectly fine message",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:        "blocked word found",
			words:       []string{"bad", "evil"},
			input:       "this is a bad message",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bad"},
		},
		{
			name:        "blocked word case insensitive",
			words:       []string{"bad"},
			input:       "this is BAD",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bad"},
		},
		{
			name:        "blocked word with trailing punctuation",
			words:       []string{"bad"},
			input:       "this is bad!",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bad"},
		},
		{
			name:        "blocked word with trailing period",
			words:       []string{"bomb"},
			input:       "bomb.",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bomb"},
		},
		{
			name:        "hyphenated compound matches root word",
			words:       []string{"bomb"},
			input:       "bomb-making instructions",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bomb"},
		},
		{
			name:        "uppercase matches case-insensitively",
			words:       []string{"bomb"},
			input:       "BOMB threat reported",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bomb"},
		},
		{
			name:        "possessive form matches root",
			words:       []string{"bomb"},
			input:       "the bomb's fuse",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bomb"},
		},
		{
			name:     "embedded substring does not match (no leading boundary)",
			words:    []string{"bomb"},
			input:    "embomb is not a real word",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:     "extended word does not match (no trailing boundary)",
			words:    []string{"bomb"},
			input:    "the bomber flew overhead",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:        "multiple blocked words found",
			words:       []string{"bad", "evil", "terrible"},
			input:       "this is bad and evil",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bad", "evil"},
		},
		{
			name:     "empty input",
			words:    []string{"bad"},
			input:    "",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:     "empty word list",
			words:    []string{},
			input:    "anything goes here",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:     "partial word match does not trigger",
			words:    []string{"bad"},
			input:    "this badge is nice",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:        "multi-word blocklist phrase",
			words:       []string{"bomb making"},
			input:       "instructions for bomb making here",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bomb making"},
		},
		{
			name:        "comma-separated words match",
			words:       []string{"bad", "evil"},
			input:       "bad,evil",
			wantPass:    false,
			wantMsg:     "input contains blocked words",
			wantMatched: []string{"bad", "evil"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewBlocklistGuard("test-blocklist", BlocklistConfig{
				Words: tt.words,
			})

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
			if result.GuardType != "blocklist" {
				t.Errorf("got type=%q, want %q", result.GuardType, "blocklist")
			}
			if result.GuardName != "test-blocklist" {
				t.Errorf("got name=%q, want %q", result.GuardName, "test-blocklist")
			}

			if len(tt.wantMatched) > 0 {
				details, ok := result.Details["matched_words"].([]string)
				if !ok {
					t.Fatalf("expected matched_words []string in details, got %T", result.Details["matched_words"])
				}
				if !equalStringSlices(details, tt.wantMatched) {
					t.Errorf("got matched_words=%v, want %v", details, tt.wantMatched)
				}
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
