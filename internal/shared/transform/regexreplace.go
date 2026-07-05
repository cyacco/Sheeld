package transform

import (
	"context"
	"regexp"

	"github.com/sheeld/sheeld/internal/shared/llm"
)

// RegexReplaceRule is one pattern → replacement rewrite.
type RegexReplaceRule struct {
	// Pattern is a Go (RE2) regular expression.
	Pattern string `json:"pattern"`

	// Replace is the replacement text; $1-style group references are
	// expanded per regexp.ReplaceAllString.
	Replace string `json:"replace"`
}

// RegexReplaceConfig holds configuration for the regex_replace transformer.
type RegexReplaceConfig struct {
	Rules []RegexReplaceRule `json:"rules"`
}

// RegexReplaceTransformer rewrites every message's content by applying each
// rule's pattern → replacement in order.
type RegexReplaceTransformer struct {
	name     string
	patterns []*regexp.Regexp
	replaces []string
}

// NewRegexReplaceTransformer creates a regex_replace transformer, compiling
// all rule patterns up front.
func NewRegexReplaceTransformer(name string, cfg RegexReplaceConfig) (*RegexReplaceTransformer, error) {
	t := &RegexReplaceTransformer{
		name:     name,
		patterns: make([]*regexp.Regexp, 0, len(cfg.Rules)),
		replaces: make([]string, 0, len(cfg.Rules)),
	}
	for _, rule := range cfg.Rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, err
		}
		t.patterns = append(t.patterns, re)
		t.replaces = append(t.replaces, rule.Replace)
	}
	return t, nil
}

func (t *RegexReplaceTransformer) Name() string { return t.name }
func (t *RegexReplaceTransformer) Type() string { return "regex_replace" }

func (t *RegexReplaceTransformer) Transform(_ context.Context, msgs []llm.Message) ([]llm.Message, error) {
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		for j, re := range t.patterns {
			out[i].Content = re.ReplaceAllString(out[i].Content, t.replaces[j])
		}
	}
	return out, nil
}
