// Package transform implements the input-transformer pipeline stage:
// sequential message rewriters that run before input guards and the LLM
// call. Transformers never reject requests — they only rewrite text; a
// transformer that errors is governed by the on_error policy (fail_closed
// aborts the request with a proxy error, fail_open skips the transformer).
package transform

import (
	"context"
	"fmt"
	"time"

	"github.com/sheeld/sheeld/internal/shared/llm"
)

// Transformer rewrites the messages of a chat request.
type Transformer interface {
	// Name returns the human-readable name of this transformer instance.
	Name() string

	// Type returns the transformer type identifier.
	Type() string

	// Transform rewrites the messages array. It must not mutate the input
	// slice; returning the input unchanged (same slice) means no-op.
	Transform(ctx context.Context, messages []llm.Message) ([]llm.Message, error)
}

// FailOpenTransformer marks a transformer whose errors should skip it
// rather than abort the request.
type FailOpenTransformer interface {
	Transformer
	FailOpen() bool
}

type failOpen struct {
	Transformer
}

// FailOpen reports that errors from this transformer skip it.
func (failOpen) FailOpen() bool { return true }

// WithFailOpen wraps a transformer so that execution errors skip it
// (recorded as errored+skipped) instead of aborting the request.
func WithFailOpen(t Transformer) Transformer {
	return failOpen{t}
}

// StepResult records one transformer's outcome for the audit log.
type StepResult struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Changed    bool   `json:"changed"`
	Errored    bool   `json:"errored,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	Message    string `json:"message,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ChainResult records the outcome of the whole transformer chain.
type ChainResult struct {
	Steps           []StepResult `json:"steps"`
	Changed         bool         `json:"changed"`
	TotalDurationMs int64        `json:"total_duration_ms"`
}

// ApplyAll runs transformers sequentially in order. A fail-open transformer
// that errors is skipped (chain continues with the messages as they were
// before it ran); any other error aborts and is returned.
func ApplyAll(ctx context.Context, ts []Transformer, msgs []llm.Message) ([]llm.Message, *ChainResult, error) {
	start := time.Now()
	chain := &ChainResult{Steps: make([]StepResult, 0, len(ts))}

	current := msgs
	for _, t := range ts {
		stepStart := time.Now()
		next, err := t.Transform(ctx, current)
		step := StepResult{
			Name:       t.Name(),
			Type:       t.Type(),
			DurationMs: time.Since(stepStart).Milliseconds(),
		}
		if err != nil {
			step.Errored = true
			step.Message = err.Error()
			if fo, ok := t.(FailOpenTransformer); ok && fo.FailOpen() {
				step.Skipped = true
				chain.Steps = append(chain.Steps, step)
				continue
			}
			chain.Steps = append(chain.Steps, step)
			chain.TotalDurationMs = time.Since(start).Milliseconds()
			return nil, chain, fmt.Errorf("transformer %q (%s) failed: %w", t.Name(), t.Type(), err)
		}
		step.Changed = !messagesEqual(current, next)
		if step.Changed {
			chain.Changed = true
		}
		chain.Steps = append(chain.Steps, step)
		current = next
	}

	chain.TotalDurationMs = time.Since(start).Milliseconds()
	return current, chain, nil
}

func messagesEqual(a, b []llm.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Transformers only rewrite role/text content; other fields
		// (tool_calls, multimodal parts) pass through untouched.
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
	}
	return true
}
