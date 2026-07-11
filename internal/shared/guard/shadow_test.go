package guard

import (
	"context"
	"testing"
)

func mock(name string, passed bool) *mockGuard {
	return &mockGuard{name: name, guardType: "mock", passed: passed}
}

func runEngine(t *testing.T, guards []Guard) *EngineResult {
	t.Helper()
	res, err := NewEngine(NewRegistry()).Run(context.Background(), guards, "input",
		EvalConfig{Criteria: CriteriaAll})
	if err != nil {
		t.Fatalf("engine error: %v", err)
	}
	return res
}

func TestShadowGuard_DoesNotBlockButIsRecorded(t *testing.T) {
	// A failing shadow guard alongside a passing enforcing guard: request passes,
	// but the shadow result is recorded with its real (failed) verdict.
	res := runEngine(t, []Guard{
		mock("enforcing", true),
		WithShadow(mock("shadow-fail", false)),
	})
	if !res.Passed {
		t.Error("expected pass: a shadow guard must not block")
	}
	var shadowResult *Result
	for _, r := range res.Results {
		if r.GuardName == "shadow-fail" {
			shadowResult = r
		}
	}
	if shadowResult == nil {
		t.Fatal("shadow guard result was not recorded")
	}
	if !shadowResult.Shadow {
		t.Error("shadow result should be marked Shadow=true")
	}
	if shadowResult.Passed {
		t.Error("shadow result should preserve its real (failed) verdict")
	}
}

func TestShadowGuard_EnforcingStillBlocks(t *testing.T) {
	// A failing enforcing guard blocks even when a shadow guard passes.
	res := runEngine(t, []Guard{
		mock("enforcing-fail", false),
		WithShadow(mock("shadow-pass", true)),
	})
	if res.Passed {
		t.Error("expected reject: the enforcing guard failed")
	}
}

func TestShadowGuard_AllShadowNeverBlocks(t *testing.T) {
	// With only shadow guards, nothing enforces, so the request always passes.
	res := runEngine(t, []Guard{
		WithShadow(mock("s1", false)),
		WithShadow(mock("s2", false)),
	})
	if !res.Passed {
		t.Error("all-shadow guards must not block")
	}
}

func TestShadowGuard_MarkerDetectedThroughWrapChain(t *testing.T) {
	// Shadow marker must be found regardless of wrap order relative to fail-open.
	g := WithFailOpen(WithShadow(mock("g", false)))
	if _, ok := guardAs[ShadowGuard](g); !ok {
		t.Error("shadow marker not detected under a fail-open wrapper")
	}
	if _, ok := guardAs[FailOpenGuard](g); !ok {
		t.Error("fail-open marker not detected")
	}
}
