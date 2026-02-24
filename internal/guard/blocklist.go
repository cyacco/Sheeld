package guard

import (
	"context"
	"strings"
	"time"
)

// BlocklistGuard checks input against a list of blocked or allowed words.
type BlocklistGuard struct {
	name  string
	words map[string]struct{}
	mode  string // "block" = reject if any word found, "allow" = reject if any word NOT found
}

// BlocklistConfig holds the configuration for a BlocklistGuard.
type BlocklistConfig struct {
	Words []string `json:"words"`
	Mode  string   `json:"mode"` // "block" (default) or "allow"
}

// NewBlocklistGuard creates a new BlocklistGuard from configuration.
func NewBlocklistGuard(name string, cfg BlocklistConfig) *BlocklistGuard {
	words := make(map[string]struct{}, len(cfg.Words))
	for _, w := range cfg.Words {
		words[strings.ToLower(strings.TrimSpace(w))] = struct{}{}
	}

	mode := cfg.Mode
	if mode == "" {
		mode = "block"
	}

	return &BlocklistGuard{
		name:  name,
		words: words,
		mode:  mode,
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

	if g.mode == "block" {
		// Block mode: fail if any blocked word is found
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

	// Allow mode: fail if input contains words NOT in the allow list
	var disallowedWords []string
	for _, word := range inputWords {
		cleaned := strings.Trim(word, ".,!?;:\"'()-[]{}…")
		if cleaned == "" {
			continue
		}
		if _, found := g.words[cleaned]; !found {
			disallowedWords = append(disallowedWords, cleaned)
		}
	}

	if len(disallowedWords) > 0 {
		return &Result{
			GuardName: g.name,
			GuardType: g.Type(),
			Passed:    false,
			Message:   "input contains words not in the allow list",
			Details:   map[string]interface{}{"disallowed_words": disallowedWords},
			Duration:  duration,
		}, nil
	}

	return &Result{
		GuardName: g.name,
		GuardType: g.Type(),
		Passed:    true,
		Message:   "all words are in the allow list",
		Duration:  duration,
	}, nil
}
