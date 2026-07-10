package guard

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// BlocklistGuard rejects input that contains any blocked term. Terms are
// matched on word boundaries (case-insensitive), so a term matches whole words
// and multi-word phrases — "ignore previous instructions" matches as a phrase,
// and "password" does not fire on "passwordless" — while regex metacharacters
// in a term are treated literally.
type BlocklistGuard struct {
	name     string
	patterns []blockPattern
}

type blockPattern struct {
	term string
	re   *regexp.Regexp
}

// BlocklistConfig holds the configuration for a BlocklistGuard.
type BlocklistConfig struct {
	Words []string `json:"words"`
}

// NewBlocklistGuard creates a new BlocklistGuard from configuration.
func NewBlocklistGuard(name string, cfg BlocklistConfig) *BlocklistGuard {
	patterns := make([]blockPattern, 0, len(cfg.Words))
	seen := make(map[string]struct{}, len(cfg.Words))
	for _, w := range cfg.Words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		key := strings.ToLower(w)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		// QuoteMeta keeps regex metacharacters literal; \b anchors to word
		// boundaries so substrings of larger words don't match. MustCompile is
		// safe because the term is fully quoted.
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`)
		patterns = append(patterns, blockPattern{term: w, re: re})
	}
	return &BlocklistGuard{name: name, patterns: patterns}
}

func (g *BlocklistGuard) Type() string { return "blocklist" }
func (g *BlocklistGuard) Name() string { return g.name }

func (g *BlocklistGuard) Validate(_ context.Context, input string) (*Result, error) {
	start := time.Now()

	var matchedWords []string
	for _, p := range g.patterns {
		if p.re.MatchString(input) {
			matchedWords = append(matchedWords, p.term)
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
