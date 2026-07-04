package guard

import (
	"context"
	"time"
)

// Guard is the interface that all guardrail implementations must satisfy.
// Each guard validates a text input and returns a result indicating pass/fail.
type Guard interface {
	// Type returns the guard type identifier (e.g., "blocklist", "regex").
	Type() string

	// Name returns the human-readable name of this guard instance.
	Name() string

	// Validate checks the input text against this guard's rules.
	// Returns a Result indicating whether the input passed or failed.
	Validate(ctx context.Context, input string) (*Result, error)
}

// Result holds the outcome of a single guard validation.
type Result struct {
	// GuardName is the human-readable name of the guard that produced this result.
	GuardName string `json:"guard_name"`

	// GuardType is the type identifier of the guard (e.g., "blocklist", "regex").
	GuardType string `json:"guard_type"`

	// Passed indicates whether the input passed this guard's validation.
	Passed bool `json:"passed"`

	// Message provides a human-readable explanation of the result.
	Message string `json:"message"`

	// Details contains guard-type-specific metadata about the validation.
	// For blocklist: {"matched_words": ["word1", "word2"]}
	// For regex: {"matched_patterns": ["\\bfoo\\b"]}
	Details map[string]interface{} `json:"details,omitempty"`

	// Duration is how long this guard took to execute.
	Duration time.Duration `json:"duration_ms"`
}

// FailOpenGuard is implemented by guards that should be treated as passed
// when they error (e.g. an external moderation API outage), instead of the
// default fail-closed behavior.
type FailOpenGuard interface {
	Guard
	FailOpen() bool
}

// failOpen wraps a guard to mark it fail-open on error.
type failOpen struct {
	Guard
}

// FailOpen reports that errors from this guard should count as passed.
func (failOpen) FailOpen() bool { return true }

// WithFailOpen wraps a guard so that execution errors count as passed
// (marked as errored in the result) rather than failing the request.
func WithFailOpen(g Guard) Guard {
	return failOpen{g}
}

// PassCriteria defines how multiple guard results are evaluated.
type PassCriteria string

const (
	// CriteriaAll requires every guard to pass.
	CriteriaAll PassCriteria = "all"

	// CriteriaAny requires at least one guard to pass.
	CriteriaAny PassCriteria = "any"

	// CriteriaNofM requires at least N guards to pass out of M total.
	CriteriaNofM PassCriteria = "n_of_m"
)

// EngineResult holds the aggregated outcome of running all guards for a phase.
type EngineResult struct {
	// Passed indicates whether the overall evaluation passed based on the criteria.
	Passed bool `json:"passed"`

	// Criteria is the pass criteria that was applied.
	Criteria PassCriteria `json:"criteria"`

	// Threshold is the N value for n_of_m criteria (nil for all/any).
	Threshold *int `json:"threshold,omitempty"`

	// Results contains the individual result from each guard.
	Results []*Result `json:"results"`

	// TotalDuration is the wall-clock time for the entire evaluation.
	TotalDuration time.Duration `json:"total_duration_ms"`

	// PassCount is how many guards passed.
	PassCount int `json:"pass_count"`

	// FailCount is how many guards failed.
	FailCount int `json:"fail_count"`
}
