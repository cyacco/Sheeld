package guard

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockGuard is a test helper that returns a predetermined result.
type mockGuard struct {
	name      string
	guardType string
	passed    bool
	err       error
	delay     time.Duration
}

func (g *mockGuard) Type() string { return g.guardType }
func (g *mockGuard) Name() string { return g.name }
func (g *mockGuard) Validate(ctx context.Context, input string) (*Result, error) {
	if g.delay > 0 {
		select {
		case <-time.After(g.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if g.err != nil {
		return nil, g.err
	}
	msg := "passed"
	if !g.passed {
		msg = "failed"
	}
	return &Result{
		GuardName: g.name,
		GuardType: g.guardType,
		Passed:    g.passed,
		Message:   msg,
		Duration:  g.delay,
	}, nil
}

func TestEngine_AllCriteria(t *testing.T) {
	tests := []struct {
		name       string
		guards     []Guard
		wantPassed bool
		wantPass   int
		wantFail   int
	}{
		{
			name: "all pass",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
				&mockGuard{name: "g3", guardType: "mock", passed: true},
			},
			wantPassed: true,
			wantPass:   3,
			wantFail:   0,
		},
		{
			name: "one fails",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
				&mockGuard{name: "g3", guardType: "mock", passed: true},
			},
			wantPassed: false,
			wantPass:   2,
			wantFail:   1,
		},
		{
			name: "all fail",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: false},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
			},
			wantPassed: false,
			wantPass:   0,
			wantFail:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(NewRegistry())
			result, err := engine.Run(context.Background(), tt.guards, "test input", EvalConfig{
				Criteria: CriteriaAll,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Passed != tt.wantPassed {
				t.Errorf("got passed=%v, want %v", result.Passed, tt.wantPassed)
			}
			if result.PassCount != tt.wantPass {
				t.Errorf("got passCount=%d, want %d", result.PassCount, tt.wantPass)
			}
			if result.FailCount != tt.wantFail {
				t.Errorf("got failCount=%d, want %d", result.FailCount, tt.wantFail)
			}
			if result.Criteria != CriteriaAll {
				t.Errorf("got criteria=%q, want %q", result.Criteria, CriteriaAll)
			}
			if len(result.Results) != len(tt.guards) {
				t.Errorf("got %d results, want %d", len(result.Results), len(tt.guards))
			}
		})
	}
}

func TestEngine_AnyCriteria(t *testing.T) {
	tests := []struct {
		name       string
		guards     []Guard
		wantPassed bool
	}{
		{
			name: "one passes is enough",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: false},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
				&mockGuard{name: "g3", guardType: "mock", passed: false},
			},
			wantPassed: true,
		},
		{
			name: "all fail",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: false},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
			},
			wantPassed: false,
		},
		{
			name: "all pass",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(NewRegistry())
			result, err := engine.Run(context.Background(), tt.guards, "test", EvalConfig{
				Criteria: CriteriaAny,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Passed != tt.wantPassed {
				t.Errorf("got passed=%v, want %v", result.Passed, tt.wantPassed)
			}
		})
	}
}

func TestEngine_NofMCriteria(t *testing.T) {
	tests := []struct {
		name       string
		guards     []Guard
		threshold  int
		wantPassed bool
	}{
		{
			name: "2 of 3 required, 2 pass",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
				&mockGuard{name: "g3", guardType: "mock", passed: true},
			},
			threshold:  2,
			wantPassed: true,
		},
		{
			name: "2 of 3 required, 1 pass",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
				&mockGuard{name: "g3", guardType: "mock", passed: false},
			},
			threshold:  2,
			wantPassed: false,
		},
		{
			name: "1 of 3 required, 1 pass",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: false},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
				&mockGuard{name: "g3", guardType: "mock", passed: false},
			},
			threshold:  1,
			wantPassed: true,
		},
		{
			name: "3 of 3 required, all pass",
			guards: []Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
				&mockGuard{name: "g3", guardType: "mock", passed: true},
			},
			threshold:  3,
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(NewRegistry())
			result, err := engine.Run(context.Background(), tt.guards, "test", EvalConfig{
				Criteria:  CriteriaNofM,
				Threshold: tt.threshold,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Passed != tt.wantPassed {
				t.Errorf("got passed=%v, want %v", result.Passed, tt.wantPassed)
			}
			if result.Threshold == nil || *result.Threshold != tt.threshold {
				t.Errorf("got threshold=%v, want %d", result.Threshold, tt.threshold)
			}
		})
	}
}

func TestEngine_EmptyGuards(t *testing.T) {
	engine := NewEngine(NewRegistry())
	result, err := engine.Run(context.Background(), []Guard{}, "test", EvalConfig{
		Criteria: CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected empty guards to pass")
	}
	if len(result.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Results))
	}
}

func TestEngine_GuardError(t *testing.T) {
	guards := []Guard{
		&mockGuard{name: "ok", guardType: "mock", passed: true},
		&mockGuard{name: "broken", guardType: "mock", err: fmt.Errorf("connection timeout")},
	}

	engine := NewEngine(NewRegistry())
	result, err := engine.Run(context.Background(), guards, "test", EvalConfig{
		Criteria: CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Guard error should be treated as a failure
	if result.Passed {
		t.Error("expected failure when a guard errors")
	}
	if result.PassCount != 1 {
		t.Errorf("got passCount=%d, want 1", result.PassCount)
	}
	if result.FailCount != 1 {
		t.Errorf("got failCount=%d, want 1", result.FailCount)
	}
}

func TestEngine_PerGuardDuration(t *testing.T) {
	// A successful guard that takes ~5ms and an erroring guard that takes
	// ~15ms should each report their own per-guard duration, not the
	// aggregate wall-clock time of the engine run.
	guards := []Guard{
		&mockGuard{name: "fast-ok", guardType: "mock", passed: true, delay: 5 * time.Millisecond},
		&mockGuard{name: "slow-err", guardType: "mock", err: fmt.Errorf("boom"), delay: 15 * time.Millisecond},
	}

	engine := NewEngine(NewRegistry())
	result, err := engine.Run(context.Background(), guards, "test", EvalConfig{
		Criteria: CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}

	var okRes, errRes *Result
	for _, r := range result.Results {
		switch r.GuardName {
		case "fast-ok":
			okRes = r
		case "slow-err":
			errRes = r
		}
	}
	if okRes == nil || errRes == nil {
		t.Fatalf("missing expected result: ok=%v err=%v", okRes, errRes)
	}

	// Successful guard should report ~5ms, well under 10ms.
	if okRes.Duration >= 10*time.Millisecond {
		t.Errorf("fast-ok guard Duration=%v, expected <10ms", okRes.Duration)
	}
	// Erroring guard should report ~15ms of its own work, well under 25ms.
	// Before the fix this was time.Since(engineStart), which would creep
	// up toward the aggregate wall-clock as more guards ran in parallel.
	if errRes.Duration >= 25*time.Millisecond {
		t.Errorf("slow-err guard Duration=%v, expected <25ms (must reflect per-guard time, not aggregate)", errRes.Duration)
	}
	// And it should still actually have measured something (sanity check).
	if errRes.Duration <= 0 {
		t.Errorf("slow-err guard Duration=%v, expected >0", errRes.Duration)
	}
}

func TestEngine_ConcurrentExecution(t *testing.T) {
	// Verify guards run concurrently by checking that total time is ~delay, not ~N*delay
	delay := 50 * time.Millisecond
	guards := []Guard{
		&mockGuard{name: "g1", guardType: "mock", passed: true, delay: delay},
		&mockGuard{name: "g2", guardType: "mock", passed: true, delay: delay},
		&mockGuard{name: "g3", guardType: "mock", passed: true, delay: delay},
	}

	engine := NewEngine(NewRegistry())
	start := time.Now()
	result, err := engine.Run(context.Background(), guards, "test", EvalConfig{
		Criteria: CriteriaAll,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected all guards to pass")
	}

	// Should take roughly 1x delay (concurrent), not 3x delay (sequential)
	maxExpected := delay * 3
	if elapsed >= maxExpected {
		t.Errorf("execution took %v, expected less than %v (guards should run concurrently)", elapsed, maxExpected)
	}
}
