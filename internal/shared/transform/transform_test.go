package transform

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

func msgs(contents ...string) []llm.Message {
	out := make([]llm.Message, len(contents))
	for i, c := range contents {
		out[i] = llm.Message{Role: "user", Content: c}
	}
	return out
}

func TestApplyAll_SequentialOrder(t *testing.T) {
	// a→b then b→c proves ordering: only sequential application yields "c".
	ts := []Transformer{
		&testReplace{name: "t1", find: "a", replace: "b"},
		&testReplace{name: "t2", find: "b", replace: "c"},
	}
	out, chain, err := ApplyAll(context.Background(), ts, msgs("a"))
	if err != nil {
		t.Fatalf("ApplyAll: %v", err)
	}
	if out[0].Content != "c" {
		t.Errorf("expected sequential result 'c', got %q", out[0].Content)
	}
	if !chain.Changed || len(chain.Steps) != 2 || !chain.Steps[0].Changed || !chain.Steps[1].Changed {
		t.Errorf("unexpected chain result: %+v", chain)
	}
}

func TestApplyAll_NoChange(t *testing.T) {
	ts := []Transformer{&testReplace{name: "t", find: "zzz", replace: "x"}}
	out, chain, err := ApplyAll(context.Background(), ts, msgs("hello"))
	if err != nil {
		t.Fatalf("ApplyAll: %v", err)
	}
	if out[0].Content != "hello" || chain.Changed || chain.Steps[0].Changed {
		t.Errorf("expected no-op, got %+v / %+v", out, chain)
	}
}

func TestApplyAll_FailClosedAborts(t *testing.T) {
	ts := []Transformer{
		&testReplace{name: "boom", err: errors.New("upstream down")},
		&testReplace{name: "never", find: "a", replace: "b"},
	}
	_, chain, err := ApplyAll(context.Background(), ts, msgs("a"))
	if err == nil {
		t.Fatal("expected error")
	}
	if len(chain.Steps) != 1 || !chain.Steps[0].Errored || chain.Steps[0].Skipped {
		t.Errorf("unexpected chain: %+v", chain)
	}
}

func TestApplyAll_FailOpenSkips(t *testing.T) {
	ts := []Transformer{
		WithFailOpen(&testReplace{name: "boom", err: errors.New("upstream down")}),
		&testReplace{name: "t2", find: "a", replace: "b"},
	}
	out, chain, err := ApplyAll(context.Background(), ts, msgs("a"))
	if err != nil {
		t.Fatalf("expected fail-open skip, got error: %v", err)
	}
	if out[0].Content != "b" {
		t.Errorf("expected chain to continue after skip, got %q", out[0].Content)
	}
	if !chain.Steps[0].Errored || !chain.Steps[0].Skipped {
		t.Errorf("expected first step errored+skipped: %+v", chain.Steps[0])
	}
}

func TestApplyAll_DoesNotMutateInput(t *testing.T) {
	original := msgs("a")
	ts := []Transformer{&testReplace{name: "t", find: "a", replace: "b"}}
	if _, _, err := ApplyAll(context.Background(), ts, original); err != nil {
		t.Fatalf("ApplyAll: %v", err)
	}
	if original[0].Content != "a" {
		t.Error("input slice was mutated")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if r.Has("test_replace") {
		t.Error("registry should start empty")
	}
	if _, err := r.Create("test_replace", "t", json.RawMessage(`{}`)); err == nil {
		t.Error("expected unknown-type error")
	}
	r.Register("test_replace", TestReplaceFactory)
	if !r.Has("test_replace") {
		t.Error("expected type registered")
	}
	if _, err := r.Create("test_replace", "t", json.RawMessage(`{"find":"x"}`)); err != nil {
		t.Errorf("unexpected create error: %v", err)
	}
	if _, err := r.Create("test_replace", "t", json.RawMessage(`{}`)); err == nil {
		t.Error("expected factory validation error for missing find")
	}
}
