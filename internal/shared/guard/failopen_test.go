package guard

import (
	"context"
	"errors"
	"testing"
)

func TestEngine_FailOpenGuard(t *testing.T) {
	engine := NewEngine(NewRegistry())
	errGuard := &mockGuard{name: "flaky", guardType: "openai_moderation", err: errors.New("upstream timeout")}

	t.Run("fail closed by default", func(t *testing.T) {
		res, err := engine.Run(context.Background(), []Guard{errGuard}, "input", EvalConfig{Criteria: CriteriaAll})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Passed {
			t.Error("expected errored guard to fail closed by default")
		}
		if res.Results[0].Details["errored"] != true {
			t.Error("expected result marked as errored")
		}
	})

	t.Run("fail open when wrapped", func(t *testing.T) {
		res, err := engine.Run(context.Background(), []Guard{WithFailOpen(errGuard)}, "input", EvalConfig{Criteria: CriteriaAll})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !res.Passed {
			t.Error("expected fail-open errored guard to pass overall")
		}
		if res.Results[0].Details["errored"] != true {
			t.Error("expected result marked as errored")
		}
		if res.PassCount != 1 {
			t.Errorf("expected pass_count 1, got %d", res.PassCount)
		}
	})

	t.Run("fail open does not mask real failures", func(t *testing.T) {
		failing := &mockGuard{name: "blocklist", guardType: "blocklist", passed: false}
		res, err := engine.Run(context.Background(), []Guard{WithFailOpen(failing)}, "input", EvalConfig{Criteria: CriteriaAll})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Passed {
			t.Error("fail_open must only apply to errors, not real guard failures")
		}
	})
}
