package guard

import (
	"context"
	"strings"
	"time"
)

// BlocklistGuard rejects input that contains any word in the blocklist.
type BlocklistGuard struct {
	name  string
	words map[string]struct{}
}

// BlocklistConfig holds the configuration for a BlocklistGuard.
type BlocklistConfig struct {
	Words []string `json:"words"`
}

// NewBlocklistGuard creates a new BlocklistGuard from configuration.
func NewBlocklistGuard(name string, cfg BlocklistConfig) *BlocklistGuard {
	words := make(map[string]struct{}, len(cfg.Words))
	for _, w := range cfg.Words {
		words[strings.ToLower(strings.TrimSpace(w))] = struct{}{}
	}

	return &BlocklistGuard{
		name:  name,
		words: words,
	}
}

func (g *BlocklistGuard) Type() string { return "blocklist" }
func (g *BlocklistGuard) Name() string { return g.name }

func (g *BlocklistGuard) Validate(_ context.Context, input string) (*Result, error) {
	start := time.Now()

	lowerInput := strings.ToLower(input)
	inputWords := strings.Fields(lowerInput)

	var matchedWords []string
	for _, word := range inputWords {
		// Strip common punctuation from word boundaries for matching
		cleaned := strings.Trim(word, ".,!?;:\"'()-[]{}…")
		if _, found := g.words[cleaned]; found {
			matchedWords = append(matchedWords, cleaned)
		}
	}

	duration := time.Since(start)

	if len(matchedWords) > 0 {
		return &Result{
			GuardName: g.name,
			GuardType: g.Type(),
			Passed:    false,
			Message:   "input contains blocked words",
			Details:   map[string]interface{}{"matched_words": matchedWords},
			Duration:  duration,
		}, nil
	}
	return &Result{
		GuardName: g.name,
		GuardType: g.Type(),
		Passed:    true,
		Message:   "no blocked words found",
		Duration:  duration,
	}, nil
}
