package transform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

// testReplace replaces Find with Replace in every message's content. It is
// the reference transformer for unit and integration tests and is NOT
// registered in any production registry — tests register it explicitly via
// Registry.Register("test_replace", TestReplaceFactory).
type testReplace struct {
	name    string
	find    string
	replace string
	err     error
}

func (t *testReplace) Name() string { return t.name }
func (t *testReplace) Type() string { return "test_replace" }

func (t *testReplace) Transform(_ context.Context, msgs []llm.Message) ([]llm.Message, error) {
	if t.err != nil {
		return nil, t.err
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		out[i].Content = strings.ReplaceAll(out[i].Content, t.find, t.replace)
	}
	return out, nil
}

// TestReplaceFactory creates test_replace transformers. Exported so the
// integration test binary can register the type.
func TestReplaceFactory(name string, config json.RawMessage) (Transformer, error) {
	var cfg struct {
		Find    string `json:"find"`
		Replace string `json:"replace"`
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, err
	}
	if cfg.Find == "" {
		return nil, errors.New("test_replace: find is required")
	}
	return &testReplace{name: name, find: cfg.Find, replace: cfg.Replace}, nil
}
