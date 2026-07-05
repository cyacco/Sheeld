package guard

import (
	"context"
	"testing"
)

// captureGuard records the input it was validated with.
type captureGuard struct {
	got string
}

func (g *captureGuard) Type() string { return "capture" }
func (g *captureGuard) Name() string { return "capture" }
func (g *captureGuard) Validate(_ context.Context, input string) (*Result, error) {
	g.got = input
	return &Result{GuardName: g.Name(), GuardType: g.Type(), Passed: true}, nil
}

func TestWithScopeAllMessages(t *testing.T) {
	t.Run("swaps input on input phase", func(t *testing.T) {
		g := &captureGuard{}
		ctx := WithCallMeta(context.Background(), CallMeta{
			Phase:           "input",
			AllMessagesText: "system: be nice\nuser: hello",
		})
		if _, err := WithScopeAllMessages(g).Validate(ctx, "hello"); err != nil {
			t.Fatal(err)
		}
		if g.got != "system: be nice\nuser: hello" {
			t.Errorf("expected all-messages text, got %q", g.got)
		}
	})

	t.Run("passes through on output phase", func(t *testing.T) {
		g := &captureGuard{}
		ctx := WithCallMeta(context.Background(), CallMeta{
			Phase:           "output",
			AllMessagesText: "should not be used",
		})
		if _, err := WithScopeAllMessages(g).Validate(ctx, "response text"); err != nil {
			t.Fatal(err)
		}
		if g.got != "response text" {
			t.Errorf("expected passthrough, got %q", g.got)
		}
	})

	t.Run("passes through without meta", func(t *testing.T) {
		g := &captureGuard{}
		if _, err := WithScopeAllMessages(g).Validate(context.Background(), "plain"); err != nil {
			t.Fatal(err)
		}
		if g.got != "plain" {
			t.Errorf("expected passthrough, got %q", g.got)
		}
	})

	t.Run("fail-open wrapper stacks outside scope wrapper", func(t *testing.T) {
		g := WithFailOpen(WithScopeAllMessages(&captureGuard{}))
		if fo, ok := g.(FailOpenGuard); !ok || !fo.FailOpen() {
			t.Error("expected outermost wrapper to expose FailOpenGuard")
		}
	})
}
