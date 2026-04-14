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
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, g := range guards {
		wg.Add(1)
		go func(idx int, guard Guard) {
			defer wg.Done()
			result, err := guard.Validate(ctx, input)
			if err != nil {
				// Record the error as a failed result
				mu.Lock()
				results[idx] = &Result{
					GuardName: guard.Name(),
					GuardType: guard.Type(),
					Passed:    false,
					Message:   fmt.Sprintf("guard error: %v", err),
					Duration:  time.Since(start),
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, g)
	}

	// Race wg.Wait() against ctx.Done() so a cancelled request returns
	// promptly instead of blocking on slow in-flight guards.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// finalResults is the snapshot returned to the caller. We copy from the
	// shared results slice under the mutex so that any goroutines still
	// running after a ctx cancellation can't race with our reads.
	finalResults := make([]*Result, len(guards))
	select {
	case <-done:
		// All guards completed normally — safe to read results directly,
		// but copy under the lock for consistency.
		mu.Lock()
		copy(finalResults, results)
		mu.Unlock()
	case <-ctx.Done():
		// Context cancelled — snapshot what we have and fill any still-pending
		// slots as cancelled. In-flight goroutines may continue running and
		// write their original slots; that's fine because we return only
		// finalResults, which they no longer reference.
		mu.Lock()
		for idx, g := range guards {
			if results[idx] != nil {
				finalResults[idx] = results[idx]
				continue
			}
			finalResults[idx] = &Result{
				GuardName: g.Name(),
				GuardType: g.Type(),
				Passed:    false,
				Message:   "cancelled by context",
				Duration:  time.Since(start),
			}
		}
		mu.Unlock()
	}

	// Count pass/fail
	passCount := 0
	failCount := 0
	for _, r := range finalResults {
		if r.Passed {
			passCount++
		} else {
			failCount++
		}
	}

	totalDuration := time.Since(start)

	// Evaluate based on criteria
	passed := evaluate(cfg, passCount, len(guards))

	engineResult := &EngineResult{
		Passed:        passed,
		Criteria:      cfg.Criteria,
		Results:       finalResults,
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
