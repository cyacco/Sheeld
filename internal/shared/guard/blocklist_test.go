package guard

import (
	"context"
	"testing"
)

func TestBlocklistGuard_BlockMode(t *testing.T) {
	tests := []struct {
		name     string
		words    []string
		input    string
		wantPass bool
		wantMsg  string
	}{
		{
			name:     "no blocked words found",
			words:    []string{"bad", "evil"},
			input:    "this is a perfectly fine message",
			wantPass: true,
			wantMsg:  "no blocked words found",
		},
		{
			name:     "blocked word found",
			words:    []string{"bad", "evil"},
			input:    "this is a bad message",
			wantPass: false,
			wantMsg:  "input contains blocked words",
		},
		{
			name:     "blocked word case insensitive",
			words:    []string{"bad"},
			input:    "this is BAD",
			wantPass: false,
			wantMsg:  "input contains blocked words",
		},
		{
			name:     "blocked word with punctuation",
			words:    []string{"bad"},
			input:    "this is bad!",
			wantPass: false,
			wantMsg:  "input contains blocked words",
		},
		{
			name:     "multiple blocked words found",
			words:    []string{"bad", "evil", "terrible"},
			input:    "this is bad and evil",
			wantPass: false,
			wantMsg:  "input contains blocked words",
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
		})
	}
}
