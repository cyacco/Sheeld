package transform

import (
	"context"
	"strings"

	"github.com/sheeld/sheeld/internal/shared/llm"
)

// DeanonymizeTransformer restores original values for placeholders recorded
// in the request State by a reversible-mode anonymizer on the input chain.
// Attach it to the output phase. With no state or no mappings it is a
// no-op, and unmatched placeholders pass through unchanged — forgetting to
// attach it fails safe (the client sees placeholders, never leaked PII).
type DeanonymizeTransformer struct {
	name string
}

// NewDeanonymizeTransformer creates a new DeanonymizeTransformer.
func NewDeanonymizeTransformer(name string) *DeanonymizeTransformer {
	return &DeanonymizeTransformer{name: name}
}

func (t *DeanonymizeTransformer) Name() string { return t.name }
func (t *DeanonymizeTransformer) Type() string { return "deanonymize" }

func (t *DeanonymizeTransformer) Transform(ctx context.Context, msgs []llm.Message) ([]llm.Message, error) {
	state, ok := StateFrom(ctx)
	if !ok || state.Len() == 0 {
		return msgs, nil
	}
	mappings := state.Mappings()
	pairs := make([]string, 0, len(mappings)*2)
	for placeholder, original := range mappings {
		pairs = append(pairs, placeholder, original)
	}
	replacer := strings.NewReplacer(pairs...)

	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		out[i].Content = replacer.Replace(out[i].Content)
	}
	return out, nil
}
