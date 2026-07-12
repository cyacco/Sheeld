package modelcatalog

import (
	"math"
	"testing"
)

func TestModels_FilterAndNonEmpty(t *testing.T) {
	all := Models("")
	if len(all) == 0 {
		t.Fatal("catalog is empty")
	}
	openai := Models("openai")
	if len(openai) == 0 {
		t.Fatal("no openai models")
	}
	for _, m := range openai {
		if m.Provider != "openai" {
			t.Errorf("filter leaked provider %q", m.Provider)
		}
	}
}

func TestCost_ExactAndPrefixMatch(t *testing.T) {
	// gpt-4o: 2.5 in / 10 out per 1M. 1M prompt + 1M completion = 12.50.
	got, ok := Cost("gpt-4o", 1_000_000, 1_000_000)
	if !ok {
		t.Fatal("gpt-4o should be priced")
	}
	if math.Abs(got-12.5) > 1e-9 {
		t.Errorf("gpt-4o cost = %v, want 12.5", got)
	}

	// A dated variant matches by prefix to the same price.
	got2, ok := Cost("gpt-4o-2024-08-06", 1_000_000, 1_000_000)
	if !ok || math.Abs(got2-12.5) > 1e-9 {
		t.Errorf("prefix match cost = %v (ok=%v), want 12.5", got2, ok)
	}
}

func TestCost_UnknownModelNotPriced(t *testing.T) {
	if _, ok := Cost("some-local-model", 100, 100); ok {
		t.Error("unknown model must not be priced")
	}
	if _, ok := Cost("", 100, 100); ok {
		t.Error("empty model must not be priced")
	}
}

func TestCost_ProportionalToTokens(t *testing.T) {
	// gpt-4o-mini: 0.15 in / 0.60 out per 1M.
	got, ok := Cost("gpt-4o-mini", 2_000_000, 500_000)
	if !ok {
		t.Fatal("gpt-4o-mini should be priced")
	}
	want := 2.0*0.15 + 0.5*0.60
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("cost = %v, want %v", got, want)
	}
}

func TestAsOf_Present(t *testing.T) {
	if AsOf() == "" {
		t.Error("as_of should be set")
	}
}
