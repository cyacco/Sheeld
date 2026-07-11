package guard

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// RegexGuard checks input against a set of regular expression patterns.
type RegexGuard struct {
	name     string
	patterns []*regexp.Regexp
	rawPats  []string
	mode     string // "block" = reject if any pattern matches, "require" = reject if any pattern doesn't match
}

// RegexConfig holds the configuration for a RegexGuard.
type RegexConfig struct {
	Patterns []string `json:"patterns"`
	Mode     string   `json:"mode"` // "block" (default) or "require"
}

// NewRegexGuard creates a new RegexGuard from configuration.
func NewRegexGuard(name string, cfg RegexConfig) (*RegexGuard, error) {
	compiled := make([]*regexp.Regexp, 0, len(cfg.Patterns))
	for _, p := range cfg.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", p, err)
		}
		compiled = append(compiled, re)
	}

	mode := cfg.Mode
	if mode == "" {
		mode = "block"
	}

	return &RegexGuard{
		name:     name,
		patterns: compiled,
		rawPats:  cfg.Patterns,
		mode:     mode,
	}, nil
}

func (g *RegexGuard) Type() string { return "regex" }
func (g *RegexGuard) Name() string { return g.name }

func (g *RegexGuard) Validate(_ context.Context, input string) (*Result, error) {
	start := time.Now()

	var matchedPatterns []string
	var unmatchedPatterns []string

	for i, re := range g.patterns {
		if re.MatchString(input) {
			matchedPatterns = append(matchedPatterns, g.rawPats[i])
		} else {
			unmatchedPatterns = append(unmatchedPatterns, g.rawPats[i])
		}
	}

	durationMs := time.Since(start).Milliseconds()

	if g.mode == "block" {
		// Block mode: fail if any pattern matches
		if len(matchedPatterns) > 0 {
			return &Result{
				GuardName:  g.name,
				GuardType:  g.Type(),
				Passed:     false,
				Message:    "input matches blocked patterns",
				Details:    map[string]interface{}{"matched_patterns": matchedPatterns},
				DurationMs: durationMs,
			}, nil
		}
		return &Result{
			GuardName:  g.name,
			GuardType:  g.Type(),
			Passed:     true,
			Message:    "no blocked patterns matched",
			DurationMs: durationMs,
		}, nil
	}

	// Require mode: fail if any pattern doesn't match
	if len(unmatchedPatterns) > 0 {
		return &Result{
			GuardName:  g.name,
			GuardType:  g.Type(),
			Passed:     false,
			Message:    "input does not match all required patterns",
			Details:    map[string]interface{}{"unmatched_patterns": unmatchedPatterns},
			DurationMs: durationMs,
		}, nil
	}

	return &Result{
		GuardName:  g.name,
		GuardType:  g.Type(),
		Passed:     true,
		Message:    "all required patterns matched",
		DurationMs: durationMs,
	}, nil
}
