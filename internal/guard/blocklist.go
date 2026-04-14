package guard

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// BlocklistGuard rejects input that contains any word in the blocklist.
//
// Matching uses regex word boundaries (\b) and is case-insensitive. This means:
//   - "bomb-making" matches "bomb" (hyphen is a word boundary)
//   - "BOMB" matches "bomb" (case-insensitive)
//   - "bomb." matches "bomb" (period is a word boundary)
//   - "bomb's" matches "bomb" (apostrophe is a word boundary)
//   - "embomb" does NOT match "bomb" (no leading word boundary)
//   - "bomber" does NOT match "bomb" (no trailing word boundary)
type BlocklistGuard struct {
	name     string
	patterns []blocklistPattern
}

type blocklistPattern struct {
	word string
	re   *regexp.Regexp
}

// BlocklistConfig holds the configuration for a BlocklistGuard.
type BlocklistConfig struct {
	Words []string `json:"words"`
}

// NewBlocklistGuard creates a new BlocklistGuard from configuration.
// Each blocklist word is pre-compiled into a case-insensitive
// word-boundary regex (`(?i)\b<escaped_word>\b`).
func NewBlocklistGuard(name string, cfg BlocklistConfig) *BlocklistGuard {
	seen := make(map[string]struct{}, len(cfg.Words))
	patterns := make([]blocklistPattern, 0, len(cfg.Words))
	for _, w := range cfg.Words {
		normalized := strings.ToLower(strings.TrimSpace(w))
		if normalized == "" {
			continue
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}

		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(normalized) + `\b`)
		if err != nil {
			// QuoteMeta-escaped input should never fail to compile, but
			// skip silently if it somehow does rather than panicking.
			continue
		}
		patterns = append(patterns, blocklistPattern{word: normalized, re: re})
	}

	return &BlocklistGuard{
		name:     name,
		patterns: patterns,
	}
}

func (g *BlocklistGuard) Type() string { return "blocklist" }
func (g *BlocklistGuard) Name() string { return g.name }

func (g *BlocklistGuard) Validate(_ context.Context, input string) (*Result, error) {
	start := time.Now()

	var matchedWords []string
	for _, p := range g.patterns {
		if p.re.MatchString(input) {
			matchedWords = append(matchedWords, p.word)
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
