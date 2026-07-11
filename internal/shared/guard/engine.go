package guard

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Engine executes a set of guards concurrently and evaluates results
// against configurable pass criteria.
type Engine struct {
	registry *Registry
}

// NewEngine creates a new guard execution engine.
func NewEngine(registry *Registry) *Engine {
	return &Engine{registry: registry}
}

// Registry returns the engine's guard registry (for registering external guard types).
func (e *Engine) Registry() *Registry {
	return e.registry
}

// EvalConfig describes how to evaluate guard results.
type EvalConfig struct {
	Criteria  PassCriteria
	Threshold int // Only used when Criteria == CriteriaNofM
}

// Run executes all provided guards concurrently against the input and
// evaluates results according to the given criteria.
func (e *Engine) Run(ctx context.Context, guards []Guard, input string, cfg EvalConfig) (*EngineResult, error) {
	if len(guards) == 0 {
		return &EngineResult{
			Passed:   true,
			Criteria: cfg.Criteria,
			Results:  []*Result{},
		}, nil
	}

	start := time.Now()

	// Run all guards concurrently
	results := make([]*Result, len(guards))
	errs := make([]error, len(guards))
	var wg sync.WaitGroup

	for i, g := range guards {
		wg.Add(1)
		go func(idx int, guard Guard) {
			defer wg.Done()
			guardStart := time.Now()
			result, err := guard.Validate(ctx, input)
			if err != nil {
				errs[idx] = fmt.Errorf("guard %q (%s) failed: %w", guard.Name(), guard.Type(), err)
				// Errors fail closed unless the guard is marked fail-open,
				// in which case an outage of the guard's dependency doesn't
				// block traffic.
				passed := false
				if fo, ok := guardAs[FailOpenGuard](guard); ok && fo.FailOpen() {
					passed = true
				}
				result = &Result{
					GuardName: guard.Name(),
					GuardType: guard.Type(),
					Passed:    passed,
					Message:   fmt.Sprintf("guard error: %v", err),
					Details:   map[string]interface{}{"errored": true},
					Duration:  time.Since(guardStart),
				}
			}
			// Shadow guards run and are recorded, but never affect the decision.
			if sg, ok := guardAs[ShadowGuard](guard); ok && sg.Shadow() {
				result.Shadow = true
			}
			results[idx] = result
		}(i, g)
	}

	wg.Wait()

	// Collect any hard errors (distinct from guard failures)
	for _, err := range errs {
		if err != nil {
			// Log but don't fail the whole engine — the guard result is already marked as failed
			_ = err
		}
	}

	// Count pass/fail among enforcing (non-shadow) guards only; shadow guards
	// are recorded but excluded from the accept/reject decision.
	passCount := 0
	failCount := 0
	enforcing := 0
	for _, r := range results {
		if r.Shadow {
			continue
		}
		enforcing++
		if r.Passed {
			passCount++
		} else {
			failCount++
		}
	}

	totalDuration := time.Since(start)

	// With no enforcing guards (all shadow), nothing can block: pass.
	passed := enforcing == 0 || evaluate(cfg, passCount, enforcing)

	engineResult := &EngineResult{
		Passed:        passed,
		Criteria:      cfg.Criteria,
		Results:       results,
		TotalDuration: totalDuration,
		PassCount:     passCount,
		FailCount:     failCount,
	}

	if cfg.Criteria == CriteriaNofM {
		engineResult.Threshold = &cfg.Threshold
	}

	return engineResult, nil
}

// evaluate determines the overall pass/fail based on criteria.
func evaluate(cfg EvalConfig, passCount, total int) bool {
	switch cfg.Criteria {
	case CriteriaAll:
		return passCount == total
	case CriteriaAny:
		return passCount > 0
	case CriteriaNofM:
		return passCount >= cfg.Threshold
	default:
		// Default to "all must pass" for unknown criteria
		return passCount == total
	}
}
